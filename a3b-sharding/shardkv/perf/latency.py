import numpy as np
import matplotlib.pyplot as plt

# Step 1: Read data from the first file
filename1 = 'perf/3C_A_latency.txt'  # Replace with your first file name
data1 = np.loadtxt(filename1)

# Step 2: Read data from the second file
filename2 = 'perf/3C_B_latency.txt'  # Replace with your second file name
data2 = np.loadtxt(filename2)

# Step 3: Read data from the third file
filename3 = 'perf/3C_C_latency.txt'  # Replace with your third file name
data3 = np.loadtxt(filename3)

# Step 4: Sort the data
sorted_data1 = np.sort(data1)
sorted_data2 = np.sort(data2)
sorted_data3 = np.sort(data3)

# Step 5: Compute the cumulative probabilities
cdf1 = np.arange(1, len(sorted_data1) + 1) / len(sorted_data1)
cdf2 = np.arange(1, len(sorted_data2) + 1) / len(sorted_data2)
cdf3 = np.arange(1, len(sorted_data3) + 1) / len(sorted_data3)

# Step 6: Plot the CDFs
plt.figure(figsize=(8, 6))
plt.plot(sorted_data1, cdf1, marker='.', linestyle='none', label='CDF - 3C_A', color='blue')
plt.plot(sorted_data2, cdf2, marker='.', linestyle='none', label='CDF - 3C_B', color='yellow')
plt.plot(sorted_data3, cdf3, marker='.', linestyle='none', label='CDF - 3C_C', color='red')
plt.title('Latency CDF - Benchmark A,B and C')
plt.xlabel('Latency in nanoseconds')
plt.ylabel('Cumulative Probability')
plt.grid()
plt.legend()
plt.show()