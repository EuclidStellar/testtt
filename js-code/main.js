class Task {
  constructor(id, description) {
    this.id = id;
    this.description = description;
    this.completed = false;
  }

  markCompleted() {
    this.completed = true;
  }
}

class TaskManager {
  constructor() {
    this.tasks = [];
    this.nextId = 1;
  }

  addTask(description) {
    const task = new Task(this.nextId++, description);
    this.tasks.push(task);
    return task;
  }

  listTasks() {
    return this.tasks.map(task => ({
      id: task.id,
      description: task.description,
      completed: task.completed
    }));
  }

  completeTask(id) {
    const task = this.tasks.find(t => t.id === id);
    if (task) task.markCompleted();
    return task;
  }
}

// Example usage
const manager = new TaskManager();
manager.addTask("Finish the report");
manager.addTask("Review pull requests");
manager.completeTask(1);

console.log("All Tasks:");
console.log(manager.listTasks());
