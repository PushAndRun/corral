from datetime import time

import img as img
import matplotlib
import numpy as np
import pandas as pd
import os
import seaborn as sns

import sklearn
from matplotlib import pyplot
from pandas.plotting._matplotlib import scatter_matrix
from sklearn.preprocessing import MinMaxScaler

os.environ['TF_CPP_MIN_LOG_LEVEL'] = '1'

np.set_printoptions(precision=3, suppress=True)

import tensorflow as tf
from tensorflow import keras
from tensorflow.keras import layers
from scikeras.wrappers import KerasRegressor
from sklearn.model_selection import GridSearchCV
import matplotlib.pyplot as plt

matplotlib.interactive(True)
pd.options.plotting.backend = 'matplotlib'



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
tasks = tasks.query('total_execution_time < 600000000000').copy()


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

print(dataset['total_execution_time'].describe())

print(dataset.describe().transpose())

columns = list(dataset.columns)

scaler = MinMaxScaler()
# fit scaler on data
scaler.fit(dataset)
# apply transform
dataset = scaler.transform(dataset)

dataset = pd.DataFrame(dataset, columns=columns)

dataset.hist(['total_execution_time'], bins=50)
pyplot.title('Total Execution Time Histogram')
pyplot.xlabel('Seconds')
pyplot.ylabel('Count')
#import seaborn as sns
#sns.boxplot(x = data['Col1'], y = data['Col2'])

#dataset.plot.box()
#import seaborn as sns
#sns.set(rc = {'figure.figsize':(15,15)})
#sns.boxplot(data=dataset)

#scatter_matrix(dataset, alpha=0.2, figsize=(15, 15), diagonal='hist')
plt.show(block=True)




#print(plt.get_backend())

#X = dataset.copy().drop(['total_execution_time'], axis=1)
#y = dataset['total_execution_time']


