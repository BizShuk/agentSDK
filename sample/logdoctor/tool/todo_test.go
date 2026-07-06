package tool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bizshuk/agentsdk/sample/logdoctor/core"
	"github.com/bizshuk/agentsdk/sample/logdoctor/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddAndListTodos(t *testing.T) {
	store := core.NewTodoStore()
	add := tool.NewAddTodo(store)
	res, err := add.Call(context.Background(), json.RawMessage(`{"title":"investigate oom"}`))
	require.NoError(t, err)
	assert.True(t, res.OK)
	out, ok := res.Output.(json.RawMessage)
	require.True(t, ok)
	var addOut tool.AddTodoOutput
	require.NoError(t, json.Unmarshal(out, &addOut))
	assert.Equal(t, "todo-1", addOut.ID)

	list := tool.NewListTodos(store)
	res2, err := list.Call(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.True(t, res2.OK)
	var listOut tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res2.Output.(json.RawMessage), &listOut))
	require.Len(t, listOut.Todos, 1)
	assert.Equal(t, "todo-1", listOut.Todos[0].ID)
	assert.Equal(t, "investigate oom", listOut.Todos[0].Title)
}

func TestCompleteTodo(t *testing.T) {
	store := core.NewTodoStore()
	add := tool.NewAddTodo(store)
	add.Call(context.Background(), json.RawMessage(`{"title":"x"}`))

	complete := tool.NewCompleteTodo(store)
	res, err := complete.Call(context.Background(), json.RawMessage(`{"id":"todo-1"}`))
	require.NoError(t, err)
	assert.True(t, res.OK)

	// List with status=open should now be empty.
	list := tool.NewListTodos(store)
	res2, _ := list.Call(context.Background(), json.RawMessage(`{"status":"open"}`))
	var out tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res2.Output.(json.RawMessage), &out))
	assert.Empty(t, out.Todos)

	// And status=done should contain the one.
	res3, _ := list.Call(context.Background(), json.RawMessage(`{"status":"done"}`))
	var out3 tool.ListTodosOutput
	require.NoError(t, json.Unmarshal(res3.Output.(json.RawMessage), &out3))
	assert.Len(t, out3.Todos, 1)
}

func TestCompleteTodoUnknownID(t *testing.T) {
	store := core.NewTodoStore()
	complete := tool.NewCompleteTodo(store)
	res, err := complete.Call(context.Background(), json.RawMessage(`{"id":"missing"}`))
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error, "todo not found")
}

func TestAddTodoSchemaMarksTitleRequired(t *testing.T) {
	store := core.NewTodoStore()
	add := tool.NewAddTodo(store)
	ts := add.Schema()
	assert.Equal(t, "add_todo", ts.Name)
	params, ok := ts.Parameters.(json.RawMessage)
	require.True(t, ok)
	var m map[string]any
	require.NoError(t, json.Unmarshal(params, &m))
	// Walk $defs — the reflector uses refs for named structs.
	reqs := []string{}
	if r := findRequired(m); len(r) > 0 {
		reqs = r
	}
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
		for _, c := range d {
			out = append(out, findRequired(c)...)
		}
	}
	return out
}