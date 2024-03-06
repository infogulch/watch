package watch

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"log/slog"

	"github.com/fsnotify/fsnotify"
)

// Watch waits for changes to any of the directories in `dirs` (recursively),
// delays for `debounce` duration until no changes occurr within the window, and
// then calls onchange. Send a value to `halt` to exit early and cancel the
// watcher. Provide an optional logger.
//
// After the first change event arrives, wait for further events until
// `debounce` delay passes with no events. This 'debounce' check tries to avoid
// a burst of reloads if multiple files are changed in quick succession (e.g.
// editor save all, or vcs checkout).
//
// After waiting, a new watcher is constructed and the old one is closed. It's
// easier to recreate the watcher from scratch than to meticulously track and
// watch/unwatch directories as their events are received. A result of this
// design is that it may not be suited to watching thousands of directories, or
// directories that change frequently.
func Watch(dirs []string, debounce time.Duration, log *slog.Logger, onchange func() bool) (halt chan<- struct{}, err error) {
	if len(dirs) == 0 {
		err = fmt.Errorf("empty watchPaths")
		return
	}
	if log == nil {
		log = slog.Default()
	}

	startwatcher := func() (*fsnotify.Watcher, error) {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("failed to create new fsnotify watcher: %w", err)
		}

		// Watch every directory under watchPaths, recursively, as recommended by `watcher.Add` docs.
		count := 0
		for _, path := range dirs {
			err = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					err = watcher.Add(path)
					count += 1
				}
				return err
			})
			if err != nil {
				watcher.Close()
				return nil, fmt.Errorf("failed scanning for directories: %w", err)
			}
		}
		log.Debug("found directories to watch", "count", count, "rootdirs", dirs)
		return watcher, nil
	}

	watcher, err := startwatcher()
	if err != nil {
		return
	}

	halt_ := make(chan struct{}, 1)

	go func() {
		var timer *time.Timer

	begin:
		select {
		case <-watcher.Events:
		case <-halt_:
			goto halt
		}
		timer = time.NewTimer(debounce)
		log.Debug("event received, debouncing", "duration", debounce)

	debounce:
		select {
		case <-watcher.Events:
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(debounce)
			goto debounce
		case <-halt_:
			goto halt
		case <-timer.C:
			// only fall through if the timer expires first
		}

		if ok := onchange(); !ok {
			goto halt
		}

		// try to rebuild watcher since there could be new subdirs.
		{
			newwatcher, err := startwatcher()
			if err != nil {
				log.Info("failed to start new fsnotify watcher", "error", err)
			} else {
				err = watcher.Close()
				if err != nil {
					log.Info("error while stopping fsnotify watcher", "error", err)
				}
				log.Debug("starting new fsnotify watcher")
				watcher = newwatcher
			}
		}
		goto begin

	halt:
		watcher.Close()
		log.Debug("watcher stopped")
	}()

	return halt_, nil
}
