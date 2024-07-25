package main

import (
	"fmt"
	"github.com/deepch/vdk/format/mp4"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"os"
	"time"
	"io/ioutil"
    "path/filepath"
)


func findOldestFile(dir string) (string, error) {
    files, err := ioutil.ReadDir(dir)
    if err != nil {
        return "", err
    }

    if len(files) != 3 {
        return "", fmt.Errorf("the directory does not contain exactly two files")
    }

    var oldestFile string
    var oldestTime time.Time

    for _, file := range files {
        if file.IsDir() {
            continue
        }

        filePath := filepath.Join(dir, file.Name())
        fileInfo, err := os.Stat(filePath)
        if err != nil {
            return "", err
        }

        if oldestFile == "" || fileInfo.ModTime().Before(oldestTime) {
            oldestFile = filePath
            oldestTime = fileInfo.ModTime()
        }
    }

    return oldestFile, nil
}

// Function to delete the file
func deleteFile(filePath string) error {
    err := os.Remove(filePath)
    if err != nil {
        return err
    }
    fmt.Printf("Deleted file: %s\n", filePath)
    return nil
}

// HTTPAPIServerStreamSaveToMP4 func
func HTTPAPIServerStreamSaveToMP4(c *gin.Context) {
	var err error

	requestLogger := log.WithFields(logrus.Fields{
		"module":  "http_save_mp4",
		"stream":  c.Param("uuid"),
		"channel": c.Param("channel"),
		"func":    "HTTPAPIServerStreamSaveToMP4",
	})

	defer func() {
		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "Close",
			}).Errorln(err)
		}
	}()

	if !Storage.StreamChannelExist(c.Param("uuid"), c.Param("channel")) {
		requestLogger.WithFields(logrus.Fields{
			"call": "StreamChannelExist",
		}).Errorln(ErrorStreamNotFound.Error())
		return
	}

	if !RemoteAuthorization("save", c.Param("uuid"), c.Param("channel"), c.Query("token"), c.ClientIP()) {
		requestLogger.WithFields(logrus.Fields{
			"call": "RemoteAuthorization",
		}).Errorln(ErrorStreamUnauthorized.Error())
		return
	}
	c.Writer.Write([]byte("await save started"))
	go func() {
		Storage.StreamChannelRun(c.Param("uuid"), c.Param("channel"))
		cid, ch, _, err := Storage.ClientAdd(c.Param("uuid"), c.Param("channel"), MSE)
		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "ClientAdd",
			}).Errorln(err.Error())
			return
		}

		defer Storage.ClientDelete(c.Param("uuid"), cid, c.Param("channel"))
		codecs, err := Storage.StreamChannelCodecs(c.Param("uuid"), c.Param("channel"))
		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "StreamCodecs",
			}).Errorln(err.Error())
			return
		}
		path := fmt.Sprintf("../SNOVA-backend/static/record/%s/%s", c.Param("uuid"), c.Param("channel"))

		err = os.MkdirAll(path, 0755)

		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "MkdirAll",
			}).Errorln(err.Error())
		}
		current_time := time.Now()
		
		oldestFile, err := findOldestFile(path)
		if err != nil {
			
		}

		if oldestFile == "" {

		} else {
			err = deleteFile(oldestFile)
			if err != nil {
				log.Fatalf("Error deleting the file: %v\n", err)
			}
		}

		file_time := fmt.Sprintf("%d-%02d-%02d-%02d-%02d-%02d", 
						current_time.Year(), current_time.Month(), current_time.Day(), 
						current_time.Hour(), current_time.Minute(), current_time.Second()) 
		f, err := os.Create(fmt.Sprintf("%s/%s.mp4", path, file_time))

		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "Create",
			}).Errorln(err.Error())
		}
		defer f.Close()

		muxer := mp4.NewMuxer(f)
		err = muxer.WriteHeader(codecs)
		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "WriteHeader",
			}).Errorln(err.Error())
			return
		}
		defer muxer.WriteTrailer()

		var videoStart bool
		controlExit := make(chan bool, 10)
		dur, err := time.ParseDuration(c.Param("duration"))
		if err != nil {
			requestLogger.WithFields(logrus.Fields{
				"call": "ParseDuration",
			}).Errorln(err.Error())
		}
		saveLimit := time.NewTimer(dur)
		noVideo := time.NewTimer(10 * time.Second)
		defer log.Println("client exit")
		for {
			select {
			case <-controlExit:
				requestLogger.WithFields(logrus.Fields{
					"call": "controlExit",
				}).Errorln("Client Reader Exit")
				return
			case <-saveLimit.C:
				requestLogger.WithFields(logrus.Fields{
					"call": "saveLimit",
				}).Errorln("Saved Limit End")
				return
			case <-noVideo.C:
				requestLogger.WithFields(logrus.Fields{
					"call": "ErrorStreamNoVideo",
				}).Errorln(ErrorStreamNoVideo.Error())
				return
			case pck := <-ch:
				if pck.IsKeyFrame {
					noVideo.Reset(10 * time.Second)
					videoStart = true
				}
				if !videoStart {
					continue
				}
				if err = muxer.WritePacket(*pck); err != nil {
					return
				}
			}
		}
	}()
}
