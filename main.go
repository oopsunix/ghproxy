package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

const (
	sizeLimit int64 = 1024 * 1024 * 1024 * 10 // 允许的文件大小，默认10GB
	host            = "0.0.0.0"               // 监听地址
	port            = 45000                   // 监听端口
)

var (
	exps = []*regexp.Regexp{
		regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)/(?:releases|archive)/.*$`),
		regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)/(?:blob|raw)/.*$`),
		regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)/(?:info|git-).*$`),
		regexp.MustCompile(`^(?:https?://)?raw\.github(?:usercontent|)\.com/([^/]+)/([^/]+)/.+?/.+$`),
		regexp.MustCompile(`^(?:https?://)?gist\.github(?:usercontent|)\.com/([^/]+)/.+?/.+$`),
		regexp.MustCompile(`^(?:https?://)?api\.github\.com/.+?/([^/]+)(?:/.*)?$`),
	}
	httpClient *http.Client
	config     *Config
	configLock sync.RWMutex
	log        = logrus.New()
)

type Config struct {
	WhiteList []string `json:"whiteList"`
	BlackList []string `json:"blackList"`
	Domain    string   `json:"domain"`
	Debug     bool     `json:"debug"`
}

func init() {
	formatter := logrus.TextFormatter{
		ForceColors:               true,
		EnvironmentOverrideColors: true,
		TimestampFormat:           "2006-01-02 15:04:05",
		FullTimestamp:             true,
	}
	log.SetFormatter(&formatter)
}

func customLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查请求路径，如果路径是 "/"，则不记录日志
		if c.Request.URL.Path == "/" {
			c.Next() // 继续处理请求，但不记录日志
			return
		}

		// 记录日志（这里使用 Gin 默认的日志格式）
		start := time.Now()
		c.Next()
		end := time.Now()
		latency := end.Sub(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		path := c.Request.URL.Path
		userAgent := c.Request.UserAgent()

		logMessage := fmt.Sprintf(
			"%15s | %3d | %8v | %s   \"%s\" | %-50s",
			clientIP, statusCode, latency.Round(time.Millisecond), method, path, userAgent,
		)

		log.Debug(logMessage)
	}
}

func main() {
	loadConfig()
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			loadConfig()
		}
	}()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// 使用自定义日志中间件
	router.Use(customLogger())

	httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          1000,
			MaxIdleConnsPerHost:   1000,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 300 * time.Second,
		},
	}

	tmpl, err := template.ParseFiles("./public/index.html")
	if err != nil {
		log.Errorf("load template error: %v\n", err)
		return
	}

	httpBase := fmt.Sprintf("%s:%d", host, port)
	log.Info("start HTTP server @ ", httpBase)

	router.GET("/", func(c *gin.Context) {
		configLock.RLock()
		domain := config.Domain
		configLock.RUnlock()

		data := struct {
			Domain string
		}{
			Domain: domain,
		}

		if err := tmpl.Execute(c.Writer, data); err != nil {
			c.String(http.StatusInternalServerError, fmt.Sprintf("Error rendering template: %v", err))
		}
	})

	router.NoRoute(handler)

	err = router.Run(fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		log.Errorf("start server error: %v\n", err)
	}
}

func handler(c *gin.Context) {
	rawPath := strings.TrimPrefix(c.Request.URL.RequestURI(), "/")

	for strings.HasPrefix(rawPath, "/") {
		rawPath = strings.TrimPrefix(rawPath, "/")
	}

	if !strings.HasPrefix(rawPath, "http") {
		c.String(http.StatusForbidden, "Invalid input.")
		return
	}

	matches := checkURL(rawPath)
	if matches != nil {
		if len(config.WhiteList) > 0 && !checkList(matches, config.WhiteList) {
			c.String(http.StatusForbidden, "Forbidden by white list.")
			return
		}
		if len(config.BlackList) > 0 && checkList(matches, config.BlackList) {
			c.String(http.StatusForbidden, "Forbidden by black list.")
			return
		}
	} else {
		c.String(http.StatusForbidden, "Invalid input.")
		return
	}

	if exps[1].MatchString(rawPath) {
		rawPath = strings.Replace(rawPath, "/blob/", "/raw/", 1)
	}

	proxy(c, rawPath)
}

func proxy(c *gin.Context, u string) {
	req, err := http.NewRequest(c.Request.Method, u, c.Request.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("server error %v", err))
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Del("Host")

	resp, err := httpClient.Do(req)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("server error %v", err))
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		size, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "Invalid Content-Length")
			return
		}

		if size > sizeLimit {
			c.String(http.StatusRequestEntityTooLarge, "File too large.")
			return
		}
	}

	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Referrer-Policy")
	resp.Header.Del("Strict-Transport-Security")

	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	if location := resp.Header.Get("Location"); location != "" {
		if checkURL(location) != nil {
			c.Header("Location", "/"+location)
		} else {
			proxy(c, location)
			return
		}
	}

	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		return
	}
}

func loadConfig() {
	log.Info("loading config...")
	file, err := os.Open("config.json")
	if err != nil {
		log.Errorf("load config error: %v\n", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {

		}
	}(file)

	var newConfig Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&newConfig); err != nil {
		log.Errorf("decod config error: %v\n", err)
		return
	}

	configLock.Lock()
	config = &newConfig
	if newConfig.Debug {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}
	configLock.Unlock()
}

func checkURL(u string) []string {
	for _, exp := range exps {
		if matches := exp.FindStringSubmatch(u); matches != nil {
			return matches[1:]
		}
	}
	return nil
}

func checkList(matches, list []string) bool {
	for _, item := range list {
		if strings.HasPrefix(matches[0], item) {
			return true
		}
	}
	return false
}
