package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

var (
	mainChannel  = make(chan int, 3)  // 主线程
	imageChannel = make(chan int, 20) // 获取图片线程
	wg           = sync.WaitGroup{}   // 用于等待所有 goroutine 结束
)

func NewDoc(tagertUrl string) (doc *goquery.Document, e error) {
	//如果有代理
	if conf.Proxy {
		//使用代理获取响应
		resp, err := GetResponse(tagertUrl)
		if err != nil {
			Error("NewDoc Error:", err)
			return goquery.NewDocument(tagertUrl)
		}
		defer func() {
			resp.Body.Close()
		}()
		if resp.StatusCode == http.StatusOK {
			return goquery.NewDocumentFromResponse(resp)
		} else {
			Error("NewDoc Error!")
		}
	}
	return goquery.NewDocument(tagertUrl)
}

// 保存指定url的资源(js/css等文件)
func url2File(saveUrl string) (doc *goquery.Document, content string) {
	if strings.HasPrefix(saveUrl, "http://") {
		Info("url2File has prefix:", saveUrl)
		return
	}
	var e error
	if doc, e = NewDoc(conf.ThemesUrl + saveUrl); e != nil {
		Error(conf.ThemesUrl, saveUrl, " url2File error:", e)
		panic(e.Error())
	}
	var _ bool
	if strings.Contains(saveUrl, ".html") {
		content, _ = doc.Html()
	} else {
		content = doc.Text()
	}

	content2File(saveUrl, content)
	return doc, content
}

// 将指定内容保存为指定文件名的文件
func content2File(fileName string, content string) {
	defer func() {
		<-imageChannel
		wg.Done()
	}()
	if strings.Contains(fileName, "?") {
		fileName = fileName[:strings.LastIndex(fileName, "?")]
	}
	//拼接保存的路径
	savePath := path.Join(path.Dir(conf.SaveFolder), fileName)
	// 已存在就不保存
	if FileExists(savePath) {
		return
	}
	Info("save file:", savePath)
	//新建保存的文件夹
	if strings.Contains(savePath, "/") {
		os.MkdirAll(savePath[:strings.LastIndex(savePath, "/")], 0775)
	}

	dstFile, e := os.Create(savePath)
	if e != nil {
		Error("content2File error:", e)
		panic(e.Error())
	}
	defer dstFile.Close()
	dstFile.WriteString(content)
}

// 保存图片
func DownImg(imageURL string) {
	defer func() {
		<-imageChannel
		wg.Done()
	}()
	if strings.Contains(imageURL, "?") {
		imageURL = imageURL[:strings.LastIndex(imageURL, "?")]
	}
	//拼接保存的路径
	savePath := path.Join(path.Dir(conf.SaveFolder), imageURL)
	// 已存在就不保存
	if FileExists(savePath) {
		return
	}
	Info("save file:", savePath)
	//新建保存的文件夹
	if strings.Contains(savePath, "/") {
		os.MkdirAll(savePath[:strings.LastIndex(savePath, "/")], 0775)
	}

	//抓取
	resp, err := GetResponse(savePath)
	if err != nil {
		Error("DownImg GetResponse error:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Error("DownImg readAll error:", err)
			return
		}
		fout, err := os.Create(savePath)
		if err != nil {
			Error("DownImg Create file error:", err)
			return
		}
		defer fout.Close()
		fout.Write(body)
	} else {
		Warn("getImg resp Status Code :", resp.StatusCode, imageURL)
	}
}

// 保存css文件中所引用的图片
func SaveImageFileFromCSS(cssUrl, cssContent string) {
	re, _ := regexp.Compile("url\\((.*?)\\)")
	all := re.FindAllString(cssContent, -1)
	for _, img := range all {
		if strings.Contains(img, ".") && !strings.Contains(img, "http") {
			//移除不需要的后缀
			if strings.Contains(img, "?") {
				img = img[:strings.Index(img, "?")]
			}
			if strings.Contains(img, "#") {
				img = img[:strings.Index(img, "#")]
			}
			cssPath := cssUrl[:strings.LastIndex(cssUrl, "/")]
			//拼接保存的路径
			savePath := path.Join(path.Dir(cssPath), path.Dir(img))
			Info("SaveImageFileFromCSS:", savePath)
			// 保存文件
			wg.Add(1)
			imageChannel <- 1
			go DownImg(savePath)
		}
	}
}

// 保存网页中引用的js和css等文件
func saveHtmlDoc(doc *goquery.Document, content string) {
	defer func() {
		<-mainChannel
		wg.Done()
	}()
	// 解析引用的css
	doc.Find("link").Each(func(i int, s *goquery.Selection) {
		cssUrl, _ := s.Attr("href")
		if !strings.HasPrefix(cssUrl, "http://") && !FileExists(cssUrl) {
			// 保存css文件
			Info("save css file:", cssUrl)
			_, cssContent := url2File(cssUrl)
			//保存css里面的图片
			SaveImageFileFromCSS(cssUrl, cssContent)
		} else {
			Warn("special cssUrl link:", cssUrl)
		}
	})
	// 解析引用的js
	doc.Find("script[src]").Each(func(i int, s *goquery.Selection) {
		scriptUrl, _ := s.Attr("src")
		if !strings.HasPrefix(scriptUrl, "http://") && !FileExists(scriptUrl) {
			// 保存js文件
			Info("save js file:", scriptUrl)
			wg.Add(1)
			imageChannel <- 1
			go url2File(scriptUrl)
		} else {
			Warn("special scriptUrl link:", scriptUrl)
		}
	})
	// 解析引用的img
	doc.Find("img[src]").Each(func(i int, s *goquery.Selection) {
		imgUrl, _ := s.Attr("src")
		// 保存文件
		Info("save image file:", imgUrl)
		wg.Add(1)
		imageChannel <- 1
		go DownImg(imgUrl)
	})
}

//主程序
func main() {
	//设置log
	SetLogInfo()
	//读取配置文件,并设置
	ReadConfig()

	Info("start!")
	//清空空的文件夹和文件
	//DeleteEmptyFile(conf.SaveFolder)
	//获取总数

	indexHtmlDoc, content := url2File(conf.IndexUrl)
	wg.Add(1)
	mainChannel <- 1
	saveHtmlDoc(indexHtmlDoc, content)
	// 获取其他页
	indexHtmlDoc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		url, _ := s.Attr("href")
		if url != "#" && url != "index.html" && strings.Contains(url, ".html") {
			// 处理其他页
			Info("save other url:", url)
			wg.Add(1)
			mainChannel <- 1
			go saveHtmlDoc(url2File(url))
		}
	})

	//等待完成
	wg.Wait()
	// 完成
	Info("finish!")
}
