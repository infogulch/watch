package watch

import (
	"os"
	"testing"
	"time"
)

func TestWatchDirs(t *testing.T) {
	_ = os.Mkdir("test", 0777)
	defer os.RemoveAll("test")

	unit := 100 * time.Millisecond
	epsilon := 10 * time.Millisecond

	changed, halt, err := WatchDirs([]string{"test"}, 3*unit, nil)
	if err != nil {
		t.Fatalf("failed to watch 'test' dir: %v", err)
	}

	{ // test delay creating a directory
		start := time.Now()

		err := os.Mkdir("test/d1", 0777)
		if err != nil {
			t.Fatalf("failed to make d1: %v", err)
		}

		<-changed

		duration := time.Since(start)
		expected := 3 * unit
		if (duration - expected).Abs() > epsilon {
			t.Errorf("wrong delay, expected %v, took %v", expected, duration)
		}
	}

	time.Sleep(epsilon) // wait for restart

	{ // test creating a file in a directory that didn't initially exist
		start := time.Now()

		file, err := os.Create("test/d1/f.txt")
		if err != nil {
			t.Fatalf("failed to create f: %v", err)
		}
		file.Close()

		<-changed

		duration := time.Since(start)
		expected := 3 * unit
		if (duration - expected).Abs() > epsilon {
			t.Errorf("wrong delay, expected %v, took %v", expected, duration)
		}
	}

	time.Sleep(epsilon)

	{ // test debounce
		start := time.Now()

		file, err := os.Create("test/a.txt")
		if err != nil {
			t.Fatalf("failed to create a: %v", err)
		}
		file.Close()

		time.Sleep(unit)

		file, err = os.Create("test/b.txt")
		if err != nil {
			t.Fatalf("failed to create b: %v", err)
		}
		file.Close()

		<-changed

		duration := time.Since(start)
		expected := 4 * unit
		if (duration - expected).Abs() > epsilon {
			t.Errorf("wrong delay, expected %v, took %v", expected, duration)
		}
	}

	time.Sleep(epsilon)

	{ // test that touching a different file doesn't do anything
		start := time.Now()

		file, err := os.Create("test.txt")
		if err != nil {
			t.Fatalf("failed to create test.txt: %v", err)
		}
		defer os.Remove("test.txt")
		file.Close()

		timer := time.NewTimer(4 * unit)
		select {
		case <-changed:
			t.Errorf("no changes in dir, should not have detected any. took %v", time.Since(start))
		case <-timer.C:
			// ok
		}
	}

	halt <- struct{}{}

	time.Sleep(epsilon)

	{ // test that halt worked
		start := time.Now()
		file, err := os.Create("test/c.txt")
		if err != nil {
			t.Fatalf("failed to create c: %v", err)
		}
		file.Close()

		_, ok := <-changed
		if ok {
			t.Errorf("got ok = true from changed channel in %v, expected false", time.Since(start))
		}
	}
}
