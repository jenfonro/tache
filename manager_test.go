package tache_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenListTeam/tache"
)

type TestTask struct {
	tache.Base
	Data string
	do   func(*TestTask) error
}

func (t *TestTask) Run() error {
	return t.do(t)
}

func TestManager_Add(t *testing.T) {
	tm := tache.NewManager[*TestTask]()
	task := &TestTask{}
	tm.Add(task)
	t.Logf("%+v", task)
}

func TestCancelRunningTask(t *testing.T) {
	tm := tache.NewManager[*TestTask](tache.WithWorks(1))
	start := make(chan struct{})
	task := &TestTask{
		do: func(task *TestTask) error {
			close(start)
			<-task.Ctx().Done()
			return task.Ctx().Err()
		},
	}
	tm.Add(task)
	<-start
	task.Cancel()
	tm.Wait()
	if task.GetState() != tache.StateFailed {
		t.Fatalf("task should be failed, got %v", task.GetState())
	}
	if !errors.Is(task.GetErr(), context.Canceled) {
		t.Fatalf("task error should be context.Canceled, got %v", task.GetErr())
	}
}

func TestCancelFinishedTask(t *testing.T) {
	tm := tache.NewManager[*TestTask]()
	task := &TestTask{
		do: func(task *TestTask) error {
			return nil
		},
	}
	tm.Add(task)
	tm.Wait()

	if task.GetState() != tache.StateSucceeded {
		t.Fatalf("task should be succeeded before cancel, got %v", task.GetState())
	}

	task.Cancel()
	if task.GetState() != tache.StateSucceeded {
		t.Fatalf("cancel should not change finished task state, got %v", task.GetState())
	}
}

func TestWithRetry(t *testing.T) {
	tm := tache.NewManager[*TestTask](tache.WithMaxRetry(3), tache.WithWorks(1))
	var num atomic.Int64
	for i := int64(0); i < 10; i++ {
		task := &TestTask{
			do: func(task *TestTask) error {
				num.Add(1)
				if num.Load() < i*3 {
					return tache.NewErr("test")
				}
				return nil
			},
		}
		tm.Add(task)
	}
	tm.Wait()
	tasks := tm.GetAll()
	for _, task := range tasks {
		t.Logf("%+v", task)
	}
}

func TestWithPersistPath(t *testing.T) {
	tm := tache.NewManager[*TestTask](tache.WithPersistPath("./test.json"))
	task := &TestTask{
		do: func(task *TestTask) error {
			return nil
		},
		Data: "haha",
	}
	tm.Add(task)
	tm.Wait()
	t.Logf("%+v", task)
	time.Sleep(4 * time.Second)
}

func TestMultiTasks(t *testing.T) {
	tm := tache.NewManager[*TestTask](tache.WithWorks(3), tache.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}))))
	var num atomic.Int64
	for i := 0; i < 100; i++ {
		tm.Add(&TestTask{
			do: func(task *TestTask) error {
				num.Add(1)
				return nil
			},
		})
	}
	tm.Wait()
	//time.Sleep(3 * time.Second)
	if num.Load() != 100 {
		t.Errorf("num error, num: %d", num.Load())
	} else {
		t.Logf("num success, num: %d", num.Load())
	}
}

func TestSetWorkersNum(t *testing.T) {
	tm := tache.NewManager[*TestTask](tache.WithMaxRetry(3), tache.WithWorks(20))
	var num atomic.Int64
	var done atomic.Int64
	pass := make(chan interface{}, 50)
	for i := int64(0); i < 100; i++ {
		task := &TestTask{
			do: func(task *TestTask) error {
				num.Add(1)
				_ = <-pass
				done.Add(1)
				return nil
			},
		}
		tm.Add(task)
	}
	time.Sleep(time.Second)
	if num.Load() != 20 || done.Load() != 0 {
		t.Errorf("error, num: %d, done: %d", num.Load(), done.Load())
	}
	tm.SetWorkersNumActive(50)
	time.Sleep(time.Second)
	if num.Load() != 50 || done.Load() != 0 {
		t.Errorf("error, num: %d, done: %d", num.Load(), done.Load())
	}
	tm.SetWorkersNumActive(30)
	for i := 0; i < 30; i++ {
		pass <- nil
	}
	time.Sleep(time.Second)
	if num.Load() != 60 || done.Load() != 30 {
		t.Errorf("error, num: %d, done: %d", num.Load(), done.Load())
	}
	close(pass)
	tm.Wait()
}
