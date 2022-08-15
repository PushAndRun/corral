#This script runs an export that can be used to examine the date in another DB management system

from datetime import time

import numpy as np
import pandas as pd
from matplotlib import pyplot as plt

np.set_printoptions(precision=3, suppress=True)
import tensorflow as tf

import tensorflow as tf

from tensorflow import keras
from tensorflow.keras import layers

import pathlib
filepath = pathlib.Path('mergedLog.csv')
filepath.parent.mkdir(parents=True, exist_ok=True)

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

job_dtypes = {'job_id': 'str', 'tpch_query_id': 'str', 'polling_strategy': 'str', 'number_of_jobs': int,
              'job_number_j': int,
              'prev_job_bytes_written': int, 'splits': int, 'split_size': int, 'map_bin_size': int,
              'reduce_bin_size': int, 'max_concurrency': int, 'backend': 'str', 'function_memory': int,
              'cache_type': 'str', 'map_complexity': 'str', 'reduce_complexity': 'str', 'job_execution_time': int,
              'experiment_note': 'str', 'map_bin_sizes': 'str', 'reduce_bin_sizes': 'str'}

task_dtypes = {'r_id': 'str', 'job_id': 'str', 'task_id': 'str', 'runtime_id': 'str', 'phase': 'str',
               'job_number_t': int, 'number_of_inputs': int, 'bin_id': int, 'bin_size': int,
               'total_execution_time': int, 'function_start_latency': int, 'function_execution_duration': int,
               'poll_latency': int,
               'number_of_premature_polls': int, 'completed': 'str', 'failed': 'str',
               'function_execution_start': 'Int64',
               'function_execution_end': 'Int64', 'final_poll_time': 'Int64'}

tasks = pd.read_csv('C:/Users/tgold/Documents/Studium/BA/Tensorflow/taskLogday1', names=task_column_names,
                    na_values='?', comment='\t', quotechar='"', skiprows=1,
                    sep=',', skipinitialspace=True, low_memory=False)

jobs = pd.read_csv('C:/Users/tgold/Documents/Studium/BA/Tensorflow/jobLogday1', names=job_column_names,
                   na_values='?', comment='\t', quotechar='"', skiprows=1,
                   sep=',', skipinitialspace=True, low_memory=False)

# remove duplicate entries that were created by binsize maps
jobs = jobs.drop(['map_bin_sizes', 'reduce_bin_sizes'], axis=1)
jobs = jobs.drop_duplicates(subset=None, keep='first', inplace=False)

# remove failed tasks
tasks = tasks.query('failed != "true"')

raw_dataset = pd.merge(jobs, tasks, how='inner', on='job_id', validate="one_to_many")

raw_dataset = raw_dataset.drop(
    ['r_id', 'function_memory', 'runtime_id', 'job_number_t', 'job_number_j', 'bin_id',
     'function_start_latency', 'poll_latency', 'number_of_premature_polls', 'function_execution_start',
     'function_execution_end', 'final_poll_time', 'completed', 'failed', 'polling_strategy', 'backend',
     'cache_type', 'experiment_note'], axis=1)

dataset = raw_dataset.copy()
dataset = dataset.dropna()

# convert categorical variables
dataset['map_complexity'] = dataset['map_complexity'].map({'1': 'MC_Eeasy', '2': 'MC_Medium', '3': 'MC_High'})
dataset['reduce_complexity'] = dataset['reduce_complexity'].map({'1': 'RC_Eeasy', '2': 'RC_Medium', '3': 'RC_High'})
dataset['phase'] = dataset['phase'].map({'0': 'Map', '1': 'Reduce'})

dataset = pd.get_dummies(dataset, columns=['map_complexity'], dtype=int, prefix='', prefix_sep='')
dataset = pd.get_dummies(dataset, columns=['reduce_complexity'], dtype=int, prefix='', prefix_sep='')
dataset = pd.get_dummies(dataset, columns=['phase'], dtype=int, prefix='', prefix_sep='')

dataset.to_csv(filepath, index=False)