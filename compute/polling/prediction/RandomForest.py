import tensorflow_decision_forests as tfdf

import os
import numpy as np
import pandas as pd
import tensorflow as tf
import math

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

def split_dataset(dataset, test_ratio=0.30):
  """Splits a panda dataframe in two."""
  test_indices = np.random.rand(len(dataset)) < test_ratio
  return dataset[~test_indices], dataset[test_indices]


train_ds_pd, test_ds_pd = split_dataset(dataset)
print("{} examples in training, {} examples for testing.".format(
    len(train_ds_pd), len(test_ds_pd)))

label='total_execution_time'

train_ds = tfdf.keras.pd_dataframe_to_tf_dataset(train_ds_pd, label=label, task=tfdf.keras.Task.REGRESSION)
test_ds = tfdf.keras.pd_dataframe_to_tf_dataset(test_ds_pd, label=label, task=tfdf.keras.Task.REGRESSION)


# Configure the model.
model_7 = tfdf.keras.RandomForestModel(task = tfdf.keras.Task.REGRESSION, max_depth=30)



# Train the model.
model_7.fit(x=train_ds)


# Evaluate the model on the test dataset.
model_7.compile(metrics=["mse"])
evaluation = model_7.evaluate(test_ds, return_dict=True)

print(evaluation)
print()
print(f"MSE: {evaluation['mse']}")
print(f"RMSE: {math.sqrt(evaluation['mse'])}")

print(model_7.summary())
# The input features
print(model_7.make_inspector().features())

# The feature importances
print(model_7.make_inspector().variable_importances())
print(model_7.make_inspector().evaluation())

import matplotlib.pyplot as plt

logs = model_7.make_inspector().training_logs()

plt.figure(figsize=(12, 4))


plt.subplot(1, 2, 1)
plt.plot([log.num_trees for log in logs], [log.evaluation.accuracy for log in logs])
plt.xlabel("Number of trees")
plt.ylabel("Accuracy (out-of-bag)")

plt.subplot(1, 2, 2)
plt.plot([log.num_trees for log in logs], [log.evaluation.loss for log in logs])
plt.xlabel("Number of trees")
plt.ylabel("Logloss (out-of-bag)")

plt.show()
