class TodoApp {
  constructor() {
    this.todos = [];
    this.idCounter = 1;
  }

  addTodo(task) {
    const newTodo = {
      id: this.idCounter++,
      task: task,
      completed: false,
    };
    this.todos.push(newTodo);
    return newTodo;
  }

  removeTodo(id) {
    const index = this.todos.findIndex(todo => todo.id === id);
    if (index !== -1) {
      this.todos.splice(index, 1);
      return true;
    }
    return false;
  }

  toggleTodo(id) {
    const todo = this.todos.find(todo => todo.id === id);
    if (todo) {
      todo.completed = !todo.completed;
      return todo;
    }
    return null;
  }

  listTodos() {
    return this.todos.map(todo => ({
      id: todo.id,
      task: todo.task,
      status: todo.completed ? '✅ Done' : '❌ Pending',
    }));
  }
}

// Example usage
const app = new TodoApp();
app.addTodo("Learn JavaScript");
app.addTodo("Build a project");
app.toggleTodo(1);
console.log(app.listTodos());
