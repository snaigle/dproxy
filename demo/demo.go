package main

import (
	"encoding/json"
	"fmt"
	"golang.org/x/net/proxy"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Println("error from recover:", r)
		}
	}()

	log.Println("start user")
	cityCode := "110000"
	resp, err := http.DefaultClient.Get("http://127.0.0.1:9090/query?cityCode=" + cityCode)
	if err != nil {
		log.Fatal("请求失败", err)
	}
	defer resp.Body.Close()
	buffer, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	result := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}{}
	err = json.Unmarshal(buffer, &result)
	if err != nil {
		panic(err)
	}
	if !result.Success {
		panic("请求失败:" + result.Message)
	}
	clientId := result.Data
	log.Println("find proxy:" + clientId)
	var myClient *http.Client
	socksUrl, _ := url.Parse(fmt.Sprintf("socks5://mucang:%v@127.0.0.1:1090", clientId))
	dialer, _ := proxy.FromURL(socksUrl, &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	})
	myClient = &http.Client{Transport: &http.Transport{Proxy: nil, Dial: dialer.Dial, TLSHandshakeTimeout: 30 * time.Second}}
	req, err := http.NewRequest("GET", "https://baidu.com", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("User-Agent", `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/63.0.3239.132 Safari/537.36`)
	resp, err = myClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	buffer, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	fmt.Println("response:")
	fmt.Println(string(buffer)[:35])

}
