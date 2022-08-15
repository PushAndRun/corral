from datetime import time

import numpy as np
import pandas as pd
import os

import sklearn
from matplotlib import pyplot as plt
from pandas.plotting._matplotlib import scatter_matrix

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
     'cache_type', 'job_execution_time', 'experiment_note', 'map_complexity','reduce_complexity'], axis=1)

dataset = raw_dataset.copy()
dataset = dataset.dropna()

dataset['total_execution_time'] = round(dataset['total_execution_time'] / 1000000000, 0)

# convert categorical variables
#dataset['map_complexity'] = dataset['map_complexity'].map({1: 'MC_Eeasy', 2: 'MC_Medium', 3: 'MC_High'})
#dataset['reduce_complexity'] = dataset['reduce_complexity'].map({1: 'RC_Eeasy', 2: 'RC_Medium', 3: 'RC_High'})
dataset['phase'] = dataset['phase'].map({0: 'Map', 1: 'Reduce'})

#dataset = pd.get_dummies(dataset, columns=['map_complexity'], dtype=int, prefix='', prefix_sep='')
#dataset = pd.get_dummies(dataset, columns=['reduce_complexity'], dtype=int, prefix='', prefix_sep='')
dataset = pd.get_dummies(dataset, columns=['phase'], dtype=int, prefix='', prefix_sep='')

# drop na values
dataset = dataset.dropna()

# shuffle
dataset = dataset.sample(frac=1)
dataset = dataset.sample(frac=1)
dataset = dataset.sample(frac=1)

scatter_matrix(dataset, alpha=0.2, figsize=(15, 15), diagonal='hist')
plt.show(block=True)

X = dataset.copy().drop(['total_execution_time'], axis=1)
y = dataset['total_execution_time']

columns = list(dataset.columns)

scaler = MinMaxScaler()
# fit scaler on data
scaler.fit(dataset)
# apply transform
dataset = scaler.transform(dataset)

dataset = pd.DataFrame(dataset, columns=columns)
print(dataset.tail())

# Normalize and create normalize layer
normalizerX = tf.keras.layers.Normalization(axis=-1)
normalizerX.adapt(X)


print(X.tail())

# split data
X_train, X_test, y_train, y_test = sklearn.model_selection.train_test_split(X, y, test_size=0.2, random_state=123,
                                                                            stratify=y)

# fix random seed for reproducibility
# seed = 7
# tf.random.set_seed(seed)

# Build model
def create_model(nodes1=64, nodes2=64, nodes3=64, activation1='relu', activation2='relu', activation3='relu', init_mode='uniform'):
    model = tf.keras.Sequential([normalizerX])
    model.add(tf.keras.layers.Dense(nodes1, activation=activation1, kernel_initializer=init_mode)) #input_dim=9
    model.add(tf.keras.layers.Dense(nodes2, activation=activation2, kernel_initializer=init_mode))
    model.add(tf.keras.layers.Dense(nodes3, activation=activation3, kernel_initializer=init_mode))

    model.add(tf.keras.layers.Dense(1))

    return model


dnn_model = KerasRegressor(build_fn=create_model, verbose=1, loss="mean_squared_logarithmic_error", metrics=["mean_absolute_error", 'mean_squared_error'])  # , optimizer=tf.keras.optimizers.Adam(0.001))

# define the dictionary that is used for the grid search
batch_size = [10]
epochs = [10]
# layers = [3]
nodes1 = [128]
nodes2 = [256]
nodes3 = [256]
learning_rate = [0.001]
#momentum = [0.0, 0.4, 0.8]
optimizer = ['Adam']  # ,'Adagrad', 'RMSprop', 'Adagrad', 'Adadelta', 'Adam', 'Adamax', 'Nadam']
activation1 = ['relu']  # ,'sigmoid','tanh', 'linear', 'sigmoid', 'hard_sigmoid', 'softplus', 'softmax', 'softplus', 'softmax', 'softsign', 'sigmoid', 'hard_sigmoid', 'relu', 'tanh', 'linear','tanh', 'softplus'
activation2 = ['relu']
activation3 = ['relu']
init_mode = ['lecun_uniform'] #['uniform', 'lecun_uniform', 'normal', 'zero', 'glorot_normal', 'glorot_uniform', 'he_normal', 'he_uniform']

param_grid = dict(model__activation1=activation1, model__activation2=activation2, model__activation3=activation3, model__init_mode = init_mode,
                  model__nodes1=nodes1, model__nodes2=nodes2, model__nodes3=nodes3, epochs=epochs,
                  batch_size=batch_size, optimizer=optimizer, optimizer__learning_rate=learning_rate)

grid = GridSearchCV(estimator=dnn_model, param_grid=param_grid, n_jobs=1, cv=3, scoring='neg_mean_absolute_error')
grid_result = grid.fit(X_train, y_train)

# summarize results
print("Best: %f using %s" % (grid_result.best_score_, grid_result.best_params_))
means = grid_result.cv_results_['mean_test_score']
stds = grid_result.cv_results_['std_test_score']
params = grid_result.cv_results_['params']
for mean, stdev, param in zip(means, stds, params):
    print("%f (%f) with: %r" % (mean, stdev, param))

best_model = grid_result.best_estimator_
best_model.set_params(loss='mean_squared_error')
print("Tested metrics of best model: %f" % (best_model.score(X_test, y_test)))

