package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

// FileLog 文件日志
func FileLog(v ...interface{}) {
	os.Mkdir("log", 0666)
	file := fmt.Sprintf("./log/%s.log", time.Now().Format("20060102"))
	f, _ := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()

	log.SetOutput(f)
	log.Println(v)
}

// UnifyLog 统一日志方法
func UnifyLog(v ...interface{}) {
	log.Println(v)
	FileLog(v)
}

// UnifyLogWithTitle 统一日志方法
func UnifyLogWithTitle(title string, v ...interface{}) {
	log.Println(title)
	UnifyLog(v)
}
