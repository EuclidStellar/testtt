package main

import (
	"fmt"
)

type Task struct {
	ID          int
	Description string
	Completed   bool
}

func (t *Task) MarkCompleted() {
	t.Completed = true
}

type TaskManager struct {
	Tasks  []Task
	NextID int
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		Tasks:  []Task{},
		NextID: 1,
	}
}

func (tm *TaskManager) AddTask(description string) Task {
	task := Task{
		ID:          tm.NextID,
		Description: description,
		Completed:   false,
	}
	tm.Tasks = append(tm.Tasks, task)
	tm.NextID++
	return task
}

func (tm *TaskManager) CompleteTask(id int) {
	for i := range tm.Tasks {
		if tm.Tasks[i].ID == id {
			tm.Tasks[i].MarkCompleted()
			break
		}
	}
}

func (tm *TaskManager) ListTasks() []Task {
	return tm.Tasks
}

func main() {
	manager := NewTaskManager()
	manager.AddTask("Finish the report")
	manager.AddTask("Review pull requests")
	manager.CompleteTask(1)

	fmt.Println("All Tasks:")
	for _, task := range manager.ListTasks() {
		fmt.Printf("ID: %d | Desc: %s | Completed: %v\n", task.ID, task.Description, task.Completed)
	}
}
