package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/minio/minio-go"
	"github.com/pquerna/ffjson/ffjson"
)

// CreateMinioClient 创建minio客户端
func CreateMinioClient() *minio.Client {
	minioClient, err := minio.New(AppConfig.Endpoint, AppConfig.AccessKeyID, AppConfig.SecretAccessKey, AppConfig.UseSSL)
	if err != nil {
		log.Print("minio.new:")
		log.Println(err)
		return nil
	}
	return minioClient
}

// GetPicV2 获取图片
func GetPicV2(bucket, pic string) *PicInfo {
	// 尝试从缓存获取图片
	key := []byte(fmt.Sprintf("%s:%s", bucket, pic))

	if data, err := cache.Get(key); err == nil {
		picInfo := new(PicInfo)

		if err := ffjson.Unmarshal(data, picInfo); err != nil {
			UnifyLog(err)
		}
		if picInfo != nil {
			return picInfo
		}
	}

	minioClient := CreateMinioClient()
	if minioClient == nil {
		return nil
	}

	reader, err := minioClient.GetObject(bucket, pic, minio.GetObjectOptions{})
	if err != nil {
		UnifyLogWithTitle("minio.get:", err)
		return nil
	}
	defer reader.Close()

	buffer := new(bytes.Buffer)
	if _, err := buffer.ReadFrom(reader); err != nil {
		UnifyLogWithTitle("buffer.read:", err)
		return ProcessPicV2(bucket, pic, minioClient)
	}

	stat, err := reader.Stat()
	if err != nil {
		UnifyLog(err)
	}

	picInfo := &PicInfo{
		Buf:  buffer.Bytes(),
		Info: stat,
	}

	if buf, err := ffjson.Marshal(picInfo); err == nil {
		cache.Set(key, buf, 60*10)
	}

	return picInfo
}

// ProcessPicV2 处理图片
func ProcessPicV2(bucket, pic string, minioClient *minio.Client) *PicInfo {
	/*
		图片名格式：原图片名.宽度-高度.裁剪模式.r旋转角度.图片格式
	*/

	file := strings.Split(pic, ".")
	if len(file) < 3 {
		return nil
	}

	// 获取原始图片
	picInfo := GetPicV2(bucket, fmt.Sprintf("%s.%s", file[0], file[len(file)-1]))
	if picInfo == nil {
		return nil
	}

	reader := bytes.NewReader(picInfo.Buf)
	img, format, err := image.Decode(reader)
	if err != nil {
		UnifyLog(err)
		return nil
	}

	var newImage *image.NRGBA
	// 处理图片
	{
		// 图片裁剪
		if strings.Contains(file[1], "-") {
			isCrop := true
			cropModel := imaging.Center
			width, height := getSize(file[1])

			if len(file) == 4 {
				cropModel, isCrop = getCropModel(file[2])
			}

			if width == 0 || height == 0 {
				isCrop = false
			}

			if isCrop {
				// 缩放加裁剪图片
				newImage = imaging.Fill(img, width, height, cropModel, imaging.Lanczos)
			} else {
				// 强制缩放图片
				newImage = imaging.Resize(img, width, height, imaging.Lanczos)
			}

			img = newImage
		}

		// 图片旋转
		rotate := file[len(file)-2]
		if strings.Contains(rotate, "r") {
			angle, err := strconv.ParseFloat(strings.Replace(rotate, "r", "", 1), 64)
			if err != nil {
				UnifyLog(err)
				return nil
			}
			newImage = imaging.Rotate(img, angle, color.White)
		}
	}

	buffer := new(bytes.Buffer)
	buffer2 := new(bytes.Buffer)
	switch format {
	case "jpeg":
		jpeg.Encode(buffer, newImage, nil)
		jpeg.Encode(buffer2, newImage, nil)
	case "png":
		png.Encode(buffer, newImage)
		png.Encode(buffer2, newImage)
	default:
		return nil
	}

	// 上传新图片
	contentType := fmt.Sprintf("image/%s", file[len(file)-1])
	if _, err := minioClient.PutObject(bucket, pic, buffer, int64(buffer.Len()), minio.PutObjectOptions{ContentType: contentType}); err != nil {
		UnifyLog(err)
		return nil
	}

	// 获取图片所属
	objInfo, err := minioClient.StatObject(bucket, pic, minio.StatObjectOptions{})
	if err != nil {
		UnifyLog(err)
		return nil
	}

	newPic := &PicInfo{
		Buf:  buffer2.Bytes(),
		Info: objInfo,
	}

	// 缓存图片
	key := []byte(fmt.Sprintf("%s:%s", bucket, pic))

	if buf, err := ffjson.Marshal(newPic); err == nil {
		cache.Set(key, buf, 60*10)
	}

	return newPic
}

func getSize(size string) (int, int) {
	array := strings.Split(size, "-")
	width, _ := strconv.Atoi(array[0])
	height, _ := strconv.Atoi(array[1])

	return width, height
}

func getCropModel(model string) (imaging.Anchor, bool) {
	switch strings.ToUpper(model) {
	case "C":
		return imaging.Center, true
	case "T":
		return imaging.Top, true
	case "TL":
		return imaging.TopLeft, true
	case "TR":
		return imaging.TopRight, true
	case "B":
		return imaging.Bottom, true
	case "BL":
		return imaging.BottomLeft, true
	case "BR":
		return imaging.BottomRight, true
	case "L":
		return imaging.Left, true
	case "R":
		return imaging.Right, true
	default:
		return imaging.Center, false
	}
}
