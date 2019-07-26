package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	syncRun()
	asyncRun()
}

func syncRun() {
	start := time.Now()
	for index := 0; index < 20; index++ {
		run()
	}
	end := time.Now()
	fmt.Printf("sync use: %s", end.Sub(start))
}

func asyncRun() {
	wg := new(sync.WaitGroup)
	wg.Add(20)

	start := time.Now()
	for index := 0; index < 20; index++ {
		go func() {
			run()
			wg.Done()
		}()
	}
	wg.Wait()
	end := time.Now()
	fmt.Printf("async use: %s", end.Sub(start))
}

func run() {
	time.Sleep(time.Second)
	fmt.Println("go go go")
}
