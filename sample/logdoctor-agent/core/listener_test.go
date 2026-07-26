package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/bizshuk/agentsdk/sample/logdoctor-agent/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogFileListenerReadsFileOnce(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.log")
	require.NoError(t, os.WriteFile(p, []byte("line1\nline2\nline3"), 0o600))

	l, err := domain.NewLogFileListener(p)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch := l.Observations(ctx)
	var got []string
	for p := range ch {
		s, _ := p.Payload.(string)
		got = append(got, s)
	}
	require.Len(t, got, 1)
	assert.Equal(t, "line1\nline2\nline3", got[0])
}

func TestLogFileListenerMissingFile(t *testing.T) {
	_, err := domain.NewLogFileListener("/nonexistent/path.log")
	assert.Error(t, err)
}