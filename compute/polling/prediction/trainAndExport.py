from datetime import time

import numpy as np
import pandas as pd
import os

import sklearn

os.environ['TF_CPP_MIN_LOG_LEVEL'] = '1'

np.set_printoptions(precision=3, suppress=True)

import tensorflow as tf
from matplotlib import pyplot
from tensorflow import keras
from tensorflow.keras import layers
from scikeras.wrappers import KerasRegressor
from sklearn.model_selection import GridSearchCV

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

print("Raw training data:")
print(dataset.tail())

X = dataset.copy().drop(['total_execution_time'], axis=1)
y = dataset['total_execution_time']

# Normalize and create normalize layer
normalizer = tf.keras.layers.Normalization(axis=-1)
normalizer.adapt(X)

# split data
X_train, X_test, y_train, y_test = sklearn.model_selection.train_test_split(X, y, test_size=0.2, random_state=123,
                                                                            stratify=y)

from datetime import datetime

now = datetime.now()

current_time = now.strftime("%H:%M:%S")
print("Current Time =", current_time)

# fix random seed for reproducibility
# seed = 7
# tf.random.set_seed(seed)

# Build model
def create_model(learning_rate=0.001, activation='relu', nodes1=128, nodes2=160, nodes3=64):  # optimizer='adam' layers=2,
    model = tf.keras.Sequential([normalizer])

    model.add(tf.keras.layers.Dense(nodes1, activation=activation))
    model.add(tf.keras.layers.Dense(nodes2, activation=activation))
    model.add(tf.keras.layers.Dense(nodes3, activation=activation))

    model.add(tf.keras.layers.Dense(1))

    model.compile(loss='mean_squared_logarithmic_error', optimizer=tf.keras.optimizers.Adam(learning_rate))

    model.summary()

    return model

dnn_model=create_model()

history=dnn_model.fit(X_train, y_train, validation_data=(X_test, y_test), epochs=50, verbose=0)

now = datetime.now()

current_time = now.strftime("%H:%M:%S")
print("Current Time =", current_time)

# plot loss during training
pyplot.title('Loss / Mean Squared Logarithmic Error')
pyplot.plot(history.history['loss'], label='train')
pyplot.plot(history.history['val_loss'], label='test')
pyplot.legend()
pyplot.show()

#save the model
#dnn_model.save('dnn_model')