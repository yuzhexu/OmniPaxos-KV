import matplotlib.pyplot as plt

# Define the values
values = [2.77551, 5.36045, 3.54137]

# Define the categories
categories = ['3C_A', '3C_B', '3C_C']

# Define the colors for each bar
colors = ['blue', 'yellow', 'red']

# Create the bar graph
plt.bar(categories, values, color=colors)

# Customize the plot
plt.xlabel('')        # Label for the x-axis
plt.ylabel('Number of requests / millisecond')            # Label for the y-axis
plt.title('Throughput benchmark')  # Title

# Show the plot
plt.show()