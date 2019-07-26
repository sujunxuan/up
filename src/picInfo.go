package main

import "github.com/minio/minio-go"

// PicInfo 图片对象
type PicInfo struct {
	Buf  []byte
	Info minio.ObjectInfo
}
