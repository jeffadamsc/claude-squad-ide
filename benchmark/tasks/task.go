package tasks

// Task defines a benchmark task that can be run with or without MCP.
type Task interface {
	// Name returns a unique identifier for the task (e.g., "symbol-find-session")
	Name() string
	// Category returns the task category (symbol, understanding, edit, crossfile)
	Category() string
	// Prompt returns the prompt to send to Claude
	Prompt() string
	// Validate checks if the output is correct (optional, return nil to skip)
	Validate(output string) error
}

// Registry holds all registered tasks.
var Registry = make(map[string]Task)

// Register adds a task to the registry.
func Register(t Task) {
	Registry[t.Name()] = t
}

// GetByCategory returns all tasks in a category.
func GetByCategory(category string) []Task {
	var tasks []Task
	for _, t := range Registry {
		if t.Category() == category {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// GetAll returns all registered tasks.
func GetAll() []Task {
	tasks := make([]Task, 0, len(Registry))
	for _, t := range Registry {
		tasks = append(tasks, t)
	}
	return tasks
}

// Categories returns the list of valid category names.
func Categories() []string {
	return []string{"symbol", "understanding", "edit", "crossfile"}
}
