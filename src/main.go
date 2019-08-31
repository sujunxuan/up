package main

import (
	"fmt"
	"github.com/minio/minio-go"
	"log"
	"mime/multipart"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/coocood/freecache"
	"github.com/dgrijalva/jwt-go"
	"github.com/julienschmidt/httprouter"
	"github.com/pquerna/ffjson/ffjson"
	"github.com/rs/cors"
	"github.com/zheng-ji/goSnowFlake"
	"golang.org/x/net/http2"
)

var (
	cache     *freecache.Cache
	snowflake *goSnowFlake.IdWorker
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	cache = freecache.NewCache(AppConfig.CacheSize * 1024 * 1024)
	iw, err := goSnowFlake.NewIdWorker(1)
	if err != nil {
		log.Fatal("snowflake", err)
	}
	snowflake = iw

	router := httprouter.New()
	router.GET("/", index)
	router.GET("/:bucket/:pic", getV2)
	router.POST("/:bucket/:token", upload)
	router.PUT("/:bucket/:token", alter)

	// 允许跨域
	//c := cors.New(cors.Options{
	//	AllowedOrigins:   []string{"*"},
	//	AllowCredentials: true,
	//})

	c := cors.AllowAll()

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", AppConfig.Port),
		Handler: c.Handler(router),
	}

	log.Printf("Listening on %d", AppConfig.Port)

	// 支持HTTP2
	if AppConfig.UseHTTP2 {
		_ = http2.ConfigureServer(&server, nil)
		log.Fatal(server.ListenAndServeTLS(AppConfig.CertPath, AppConfig.KeyPath))
	} else {
		log.Fatal(server.ListenAndServe())
	}
}

func index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	_, _ = fmt.Fprint(w, "Thank you use up.\n")
}

func getV2(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	pic := GetPicV2(ps.ByName("bucket"), ps.ByName("pic"))
	if pic == nil {
		w.WriteHeader(404)
		return
	}

	w.Header().Set("Cache-Control", "max-age=31536000")
	w.Header().Set("Content-Length", strconv.FormatInt(pic.Info.Size, 10))
	w.Header().Set("Content-Type", pic.Info.ContentType)
	w.Header().Set("ETag", pic.Info.ETag)
	_, _ = w.Write(pic.Buf)
}

func upload(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	isAuth := auth(ps.ByName("token"))
	if !isAuth {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("401 token invalid"))
		return
	}

	max, _ := strconv.ParseInt(r.Header.Get("content-length"), 10, 64)
	_ = r.ParseMultipartForm(max)

	files := r.MultipartForm.File["files"]
	if len(files) < 1 {
		w.WriteHeader(400)
		return
	}

	isAllow := fileTypeCheck(files)
	if !isAllow {
		w.WriteHeader(415)
		_, _ = w.Write([]byte("415 file types are not allowed"))
		return
	}

	isNotTooBig := fileSizeCheck(max, len(files))
	if !isNotTooBig {
		w.WriteHeader(413)
		_, _ = w.Write([]byte("413 file too large"))
		return
	}

	minioClient := CreateMinioClient()
	if minioClient == nil {
		w.WriteHeader(500)
		return
	}

	bucket := ps.ByName("bucket")

	// 判断bucket是否存在，不存在则创建
	{
		found, err := minioClient.BucketExists(bucket)
		if err != nil {
			log.Println(err)
			FileLog(err)
			w.WriteHeader(500)
			return
		}

		if !found {
			if err := minioClient.MakeBucket(ps.ByName("bucket"), ""); err != nil {
				log.Println(err)
				FileLog(err)
				w.WriteHeader(500)
				return
			}
		}
	}

	// 多线程上传至minio
	var list []map[string]string
	{
		c := make(chan string, len(files))

		for _, file := range files {
			go func(head *multipart.FileHeader) {

				id, err := snowflake.NextId()
				if err != nil {
					log.Println(err)
					FileLog(err)
					w.WriteHeader(500)
					return
				}

				src, err := head.Open()
				if err != nil {
					log.Println(err)
					FileLog(err)
					w.WriteHeader(500)
					return
				}

				oldFilename := strings.Split(head.Filename, ".")
				filename := fmt.Sprintf("%d.%s", id, oldFilename[len(oldFilename)-1])
				contentType := head.Header.Get("content-type")

				if _, err := minioClient.PutObject(ps.ByName("bucket"), filename, src, head.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
					log.Println(err)
					FileLog(err)
					w.WriteHeader(500)
					return
				}

				log.Printf("%s upload.", filename)
				c <- filename
			}(file)
		}

		for f := range c {
			list = append(list, map[string]string{
				"file": fmt.Sprintf("%s/%s", bucket, f),
			})

			if len(list) == len(files) {
				close(c)
			}
		}
	}

	json, err := ffjson.Marshal(list)
	if err != nil {
		log.Println(err)
		FileLog(err)
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(json)
}

func alter(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	isAuth := auth(ps.ByName("token"))
	if !isAuth {
		w.WriteHeader(401)
		_, _ = w.Write([]byte("401 token invalid"))
		return
	}
}

// 授权验证
func auth(token string) bool {
	tk, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return []byte(AppConfig.JWTSecret), nil
	})

	if err != nil {
		return false
	}

	return tk.Valid
}

// 文件大小检查
func fileSizeCheck(size int64, count int) bool {
	limit := int64(AppConfig.FileSizeLimit * count * 1024 * 1024)
	return limit > size
}

// 文件类型检查
func fileTypeCheck(files []*multipart.FileHeader) bool {
	for _, file := range files {
		fileName := strings.Split(file.Filename, ".")
		fileType := fileName[len(fileName)-1]
		if !strings.Contains(AppConfig.FileTypes, strings.ToLower(fileType)) {
			return false
		}
	}
	return true
}
