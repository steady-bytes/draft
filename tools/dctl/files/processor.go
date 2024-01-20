package files

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

type Processor interface {
	Start(path string) error
	Wait()
}

type processor struct {
	wg   sync.WaitGroup
	jobs chan string
}

func NewProcessor(processorFunc func(filePath string)) Processor {

	p := &processor{
		wg:   sync.WaitGroup{},
		jobs: make(chan string),
	}

	for w := 1; w <= runtime.NumCPU(); w++ {
		go p.loopFilesWorker(processorFunc)
	}

	return p
}

func (p *processor) loopFilesWorker(fileProcessor func(filePath string)) error {
	for path := range p.jobs {
		files, err := os.ReadDir(path)
		if err != nil {
			p.wg.Done()
			return err
		}

		for _, file := range files {
			if !file.IsDir() {
				fileProcessor(filepath.Join(path, file.Name()))
			}
		}
		p.wg.Done()
	}
	return nil
}

func (p *processor) Start(path string) error {
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	// Add this path as a job to the workers
	// You must call it in a go routine, since if every worker is busy, then you have to wait for the channel to be free.
	p.wg.Add(1)
	go func() {
		p.jobs <- path
	}()
	for _, file := range files {
		if file.IsDir() {
			// Recursively go further in the tree
			p.Start(filepath.Join(path, file.Name()))
		}
	}
	return nil
}

func (p *processor) Wait() {
	p.wg.Wait()
}
