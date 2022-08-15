from copy import deepcopy

import numpy as np
import pandas as pd
import os

import sklearn
from keras.dtensor.integration_test_utils import train_step
from matplotlib import pyplot as plt

os.environ['TF_CPP_MIN_LOG_LEVEL'] = '1'

np.set_printoptions(precision=3, suppress=True)

import tensorflow as tf
from tensorflow import keras
from tensorflow.keras import layers
from scikeras.wrappers import KerasRegressor
from sklearn.model_selection import GridSearchCV
from sklearn.preprocessing import MinMaxScaler

job_column_names = ['job_id', 'tpch_query_id', 'polling_strategy', 'number_of_jobs', 'job_number_j',
                    'prev_job_bytes_written', 'splits', 'split_size', 'map_bin_size',
                    'reduce_bin_size', 'max_concurrency', 'backend', 'function_memory',
                    'cache_type', 'map_complexity', 'reduce_complexity', 'job_execution_time',
                    'experiment_note', 'map_bin_sizes', 'reduce_bin_sizes']

task_column_names = ['r_id', 'job_id', 'task_id', 'runtime_id', 'phase',
                     'job_number_t', 'number_of_inputs', 'bin_id', 'bin_size',
                     'total_execution_time', 'function_start_latency', 'function_execution_duration', 'poll_latency',
                     'number_of_premature_polls', 'completed', 'failed', 'function_execution_start',
                     'function_execution_end', 'final_poll_time']

tasks = pd.read_csv('./taskLog.csv',
                    names=task_column_names,
                    na_values='?', comment='\t', quotechar='"', skiprows=1,
                    sep=',', skipinitialspace=True, low_memory=False)

jobs = pd.read_csv('./jobLog.csv',
                   names=job_column_names,
                   na_values='?', comment='\t', quotechar='"', skiprows=1,
                   sep=',', skipinitialspace=True, low_memory=False)

# remove duplicates and entries that were created by binsize maps
jobs = jobs.drop(['map_bin_sizes', 'reduce_bin_sizes'], axis=1)
jobs = jobs.drop_duplicates(subset=None, keep='first', inplace=False)

# remove failed tasks
tasks = tasks.query('failed != "true"').copy()
tasks = tasks.query('total_execution_time > 0').copy()
tasks = tasks.query('total_execution_time < 200000000000').copy()

raw_dataset = pd.merge(jobs, tasks, how='inner', on='job_id', validate="one_to_many")

raw_dataset = raw_dataset.drop(
    ['r_id', 'job_id', 'task_id', 'function_memory', 'runtime_id', 'job_number_t', 'job_number_j', 'bin_id',
     'function_start_latency',
     'function_execution_duration', 'poll_latency', 'number_of_premature_polls', 'function_execution_start',
     'function_execution_end', 'final_poll_time', 'completed', 'failed', 'tpch_query_id', 'polling_strategy', 'backend',
     'cache_type', 'job_execution_time', 'experiment_note'], axis=1)

dataset = raw_dataset.copy()
dataset = dataset.dropna()

dataset['total_execution_time'] = round(dataset['total_execution_time'] / 1000000000, 0)

# convert categorical variables
dataset['map_complexity'] = dataset['map_complexity'].map({'1': 'MC_Eeasy', '2': 'MC_Medium', '3': 'MC_High'})
dataset['reduce_complexity'] = dataset['reduce_complexity'].map({'1': 'RC_Eeasy', '2': 'RC_Medium', '3': 'RC_High'})
dataset['phase'] = dataset['phase'].map({'0': 'Map', '1': 'Reduce'})

dataset = pd.get_dummies(dataset, columns=['map_complexity'], dtype=int, prefix='', prefix_sep='')
dataset = pd.get_dummies(dataset, columns=['reduce_complexity'], dtype=int, prefix='', prefix_sep='')
dataset = pd.get_dummies(dataset, columns=['phase'], dtype=int, prefix='', prefix_sep='')

# drop na values
dataset = dataset.dropna()

# shuffle
dataset = dataset.sample(frac=1)
dataset = dataset.sample(frac=1)
dataset = dataset.sample(frac=1)

X = dataset.copy().drop(['total_execution_time'], axis=1)
y = dataset['total_execution_time']


scaler = MinMaxScaler()
# fit scaler on data
scaler.fit(dataset)
# apply transform
dataset = scaler.transform(dataset)

print(X.tail())

# split data
X_train, X_test, y_train, y_test = sklearn.model_selection.train_test_split(X, y, test_size=0.2, random_state=123,
                                                                            stratify=y)

flipped_x = deepcopy(X_train)
flipped_y = deepcopy(y_train)

N = flipped_x.shape[0]
dataset = tf.data.Dataset \
    .from_tensor_slices((flipped_x, flipped_y)) \
    .shuffle(N).batch(N)

# In our toy example, we have single input feature
l = 9
# Number of gaussians to represent the multimodal distribution
k = 26

# Network
input = tf.keras.Input(shape=(l,))
layer = tf.keras.layers.Dense(50, activation='tanh', name='baselayer')(input)
mu = tf.keras.layers.Dense((l * k), activation=None, name='mean_layer')(layer)
# variance (should be greater than 0 so we exponentiate it)
var_layer = tf.keras.layers.Dense(k, activation=None, name='dense_var_layer')(layer)
var = tf.keras.layers.Lambda(lambda x: tf.math.exp(x), output_shape=(k,), name='variance_layer')(var_layer)
# mixing coefficient should sum to 1.0
pi = tf.keras.layers.Dense(k, activation='softmax', name='pi_layer')(layer)

model = tf.keras.models.Model(input, [pi, mu, var])
optimizer = tf.keras.optimizers.Adam()
model.summary()

# Take a note how easy it is to write the loss function in
# new tensorflow eager mode (debugging the function becomes intuitive too)

def calc_pdf(y, mu, var):
    """Calculate component density"""
    value = tf.subtract(y, mu)**2
    value = (1/tf.math.sqrt(2 * np.pi * var)) * tf.math.exp((-1/(2*var)) * value)
    return value

def mdn_loss(y_true, pi, mu, var):
    """MDN Loss Function
    The eager mode in tensorflow 2.0 makes is extremely easy to write
    functions like these. It feels a lot more pythonic to me.
    """
    out = calc_pdf(y_true, mu, var)
    # multiply with each pi and sum it
    out = tf.multiply(out, pi)
    out = tf.reduce_sum(out, 1, keepdims=True)
    out = -tf.math.log(out + 1e-10)
    return tf.reduce_mean(out)

@tf.function
def train_step(model, optimizer, train_x, train_y):
    # GradientTape: Trace operations to compute gradients
    with tf.GradientTape() as tape:
        pi_, mu_, var_ = model(train_x, training=True)
        # calculate loss
        loss = mdn_loss(train_y, pi_, mu_, var_)
    # compute and apply gradients
    gradients = tape.gradient(loss, model.trainable_variables)
    optimizer.apply_gradients(zip(gradients, model.trainable_variables))
    return loss

losses = []
EPOCHS = 10
print_every = int(0.1 * EPOCHS)

# Define model and optimizer
model = tf.keras.models.Model(input, [pi, mu, var])
optimizer = tf.keras.optimizers.Adam()

# Start training
print('Print every {} epochs'.format(print_every))
for i in range(EPOCHS):
    for train_x, train_y in dataset:
        loss = train_step(model, optimizer, train_x, train_y)
        losses.append(loss)
    if i % print_every == 0:
        print('Epoch {}/{}: loss {}'.format(i, EPOCHS, losses[-1]))


# Let's plot the training loss
plt.plot(range(len(losses)), losses)
plt.xlabel('Epochs')
plt.ylabel('Loss')
plt.title('Training loss')
plt.show()