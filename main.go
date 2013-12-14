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
	mainChannel  = make(chan int, 1) // 主线程
	imageChannel = make(chan int, 5) // 获取图片线程
	wg           = sync.WaitGroup{}  // 用于等待所有 goroutine 结束
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
			Info("new Doc down!")
		}()
		if resp.StatusCode == http.StatusOK {
			return goquery.NewDocumentFromResponse(resp)
		} else {
			Warn("NewDoc Error Status Code :", resp.StatusCode, tagertUrl)
		}
	}
	return goquery.NewDocument(tagertUrl)
}

// 将指定内容保存为指定文件名的文件
func saveFile(fileName string) (content string) {
	if strings.Contains(fileName, "?") {
		fileName = fileName[:strings.LastIndex(fileName, "?")]
	}
	//拼接保存的路径
	savePath := path.Join(conf.SaveFolder, fileName)
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
	resp, err := GetResponse(conf.ThemesUrl + fileName)
	if err != nil {
		Error("saveFile GetResponse error:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Error("saveFile readAll error:", err)
			return
		}
		fout, err := os.Create(savePath)
		if err != nil {
			Error("saveFile Create file error:", err)
			return
		}
		defer fout.Close()
		fout.Write(body)
		content = string(body)
		Debug("saveFile content:", content)
	} else {
		Warn("saveFile resp Status Code :", resp.StatusCode, savePath)
	}
	return
}

// 保存图片
func DownImg(imageURL string) {
	defer func() {
		//<-imageChannel
		//wg.Done()
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
	resp, err := GetResponse(conf.ThemesUrl + imageURL)
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
		Info("SaveImageFileFromCSS begin:", img)
		if strings.Contains(img, ".") && !strings.Contains(img, "http") {
			// 提取url
			Info("SaveImageFileFromCSS img:", img)
			img = strings.Replace(strings.Replace(img, "'", "", -1), "\"", "", -1)
			Info("SaveImageFileFromCSSaaa:", img)
			img = img[4:strings.Index(img, ")")]
			Info("SaveImageFileFromCSSbbb:", img)
			//移除不需要的后缀
			if strings.Contains(img, "?") {
				img = img[:strings.Index(img, "?")]
			}
			if strings.Contains(img, "#") {
				img = img[:strings.Index(img, "#")]
			}
			cssPath := cssUrl[:strings.LastIndex(cssUrl, "/")]
			Info("SaveImageFileFromCSScssPath:", cssPath)
			Info("SaveImageFileFromCSSimgPath:", img)
			//拼接保存的路径
			savePath := path.Join(cssPath, img)
			Info("SaveImageFileFromCSS:", savePath)
			// 保存文件
			//wg.Add(1)
			//imageChannel <- 1
			go DownImg(savePath)
		}
	}
}

// 保存网页中引用的js和css等文件
func saveHtmlDoc(doc *goquery.Document) {
	// 解析引用的css
	Info("saveHtmlDoc doc:", doc)
	doc.Find("link").Each(func(i int, s *goquery.Selection) {
		cssUrl, _ := s.Attr("href")
		if !strings.HasPrefix(cssUrl, "http://") && !FileExists(cssUrl) {
			// 保存css文件
			Info(i, "save css file:", cssUrl)
			//wg.Add(1)
			//imageChannel <- 1
			cssContent := saveFile(cssUrl)
			Debug("css content:", cssContent)
			//保存css里面的图片
			SaveImageFileFromCSS(cssUrl, cssContent)
		} else {
			Warn(i, "special cssUrl link:", cssUrl)
		}
	})
	Info("all link")
	// 解析引用的js
	doc.Find("script[src]").Each(func(i int, s *goquery.Selection) {
		scriptUrl, _ := s.Attr("src")
		if !strings.HasPrefix(scriptUrl, "http://") && !FileExists(scriptUrl) {
			// 保存js文件
			Info(i, "save js file:", scriptUrl)
			//wg.Add(1)
			//imageChannel <- 1
			go saveFile(scriptUrl)
		} else {
			Warn(i, "special scriptUrl link:", scriptUrl)
		}
	})
	Info("all script")
	// 解析引用的img
	doc.Find("img[src]").Each(func(i int, s *goquery.Selection) {
		Info("image src:", s)
		imgUrl, _ := s.Attr("src")
		// 保存文件
		Info(i, "save image file:", imgUrl)
		//wg.Add(1)
		//imageChannel <- 1
		go DownImg(imgUrl)
	})
	defer Info("img done")
}

//主程序
func main() {
	//设置log
	SetLogInfo()

	//读取配置文件,并设置
	ReadConfig()

	//清空空的文件夹和文件
	//DeleteEmptyFile(conf.SaveFolder)
	Info("start!")
	var e error
	var doc *goquery.Document
	saveFile(conf.IndexUrl)
	if doc, e = NewDoc(conf.ThemesUrl + conf.IndexUrl); e != nil {
		Error(conf.ThemesUrl+conf.IndexUrl, " url2File error:", e)
		panic(e.Error())
	}
	Info("aaa")
	Info(doc)
	Info("ccc")
	saveHtmlDoc(doc)
	Info("dddd")
	Info("eee")
	// 获取其他页
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		Info(i, "main doc,each:", s)
		url, _ := s.Attr("href")
		Info("main save a:", url)
		if url != "#" && url != "index.html" && strings.Contains(url, ".html") {
			// 处理其他页
			Info("main save other url:", url)
			var e error
			var doc *goquery.Document
			saveFile(url)
			if doc, e = NewDoc(conf.ThemesUrl + url); e != nil {
				Error(url, " main error:", e)
				panic(e.Error())
			}
			saveHtmlDoc(doc)
		} else {
			Warn("main save a:", url)
		}
	})
	Info("waiting finish!")
	//等待完成
	wg.Wait()
	// 完成
	Info("finish!")
}
