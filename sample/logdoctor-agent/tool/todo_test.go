package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/core"
	logdoctorcore "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor-agent/tool"
	sdktool "github.com/bizshuk/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddAndListTodos(t *testing.T) {
	store := logdoctorcore.NewTodoStore()
	reg := sdktool.NewRegistry()
	tool.NewAddTodo(store).Register(reg)
	tool.NewListTodos(store).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{ID: "c1", Name: "add_todo", Args: map[string]any{"title": "investigate oom"}})
	assert.True(t, res.OK)
	out, ok := res.Output.(json.RawMessage)
	require.True(t, ok)
	var addOut tool.AddTodoOutput
	require.NoError(t, json.Unmarshal(out, &addOut))
	assert.Equal(t, "todo-1", addOut.ID)

	res2 := reg.Call(context.Background(), core.ToolCall{ID: "c2", Name: "list_todos", Args: map[string]any{}})
	assert.True(t, res2.OK)
	var listOut tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res2.Output.(json.RawMessage), &listOut))
	require.Len(t, listOut.Todos, 1)
	assert.Equal(t, "todo-1", listOut.Todos[0].ID)
	assert.Equal(t, "investigate oom", listOut.Todos[0].Title)
}

func TestCompleteTodo(t *testing.T) {
	store := logdoctorcore.NewTodoStore()
	reg := sdktool.NewRegistry()
	tool.NewAddTodo(store).Register(reg)
	tool.NewCompleteTodo(store).Register(reg)
	tool.NewListTodos(store).Register(reg)

	reg.Call(context.Background(), core.ToolCall{ID: "c1", Name: "add_todo", Args: map[string]any{"title": "x"}})

	res := reg.Call(context.Background(), core.ToolCall{ID: "c2", Name: "complete_todo", Args: map[string]any{"id": "todo-1"}})
	assert.True(t, res.OK)

	// List with status=open should now be empty.
	res2 := reg.Call(context.Background(), core.ToolCall{ID: "c3", Name: "list_todos", Args: map[string]any{"status": "open"}})
	var out tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res2.Output.(json.RawMessage), &out))
	assert.Empty(t, out.Todos)

	// And status=done should contain the one.
	res3 := reg.Call(context.Background(), core.ToolCall{ID: "c4", Name: "list_todos", Args: map[string]any{"status": "done"}})
	var out3 tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res3.Output.(json.RawMessage), &out3))
	assert.Len(t, out3.Todos, 1)
}

func TestCompleteTodoUnknownID(t *testing.T) {
	store := logdoctorcore.NewTodoStore()
	reg := sdktool.NewRegistry()
	tool.NewCompleteTodo(store).Register(reg)

	res := reg.Call(context.Background(), core.ToolCall{ID: "c1", Name: "complete_todo", Args: map[string]any{"id": "missing"}})
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "todo not found")
}

func TestAddTodoSchemaMarksTitleRequired(t *testing.T) {
	store := logdoctorcore.NewTodoStore()
	reg := sdktool.NewRegistry()
	tool.NewAddTodo(store).Register(reg)

	tl, ok := reg.Get("add_todo")
	require.True(t, ok)
	ts := tl.Spec()
	assert.Equal(t, "add_todo", ts.Name)
	params, ok := ts.Parameters.(json.RawMessage)
	require.True(t, ok)
	var m map[string]any
	require.NoError(t, json.Unmarshal(params, &m))
	reqs := findRequired(m)
	assert.Contains(t, reqs, "title")
}

func findRequired(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	if r, ok := m["required"].([]any); ok {
		for _, x := range r {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	if d, ok := m["$defs"].(map[string]any); ok {
		for _, sub := range d {
			out = append(out, findRequired(sub)...)
		}
	}
	return out
}
