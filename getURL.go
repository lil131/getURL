package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
)

// Profile structure for request statics
type Profile struct {
	code int
	time int64
	size int
}

var myURL = "my-worker.linluliuse.workers.dev"
var method, protocol, header, tail = "GET ", " HTTP/1.1\n", "Host: ", "\r\n\r\n"

func main() {
	var url string
	var times string
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "url",
				Usage:       "Send HTTP GET request to `URL` and print out the response",
				Destination: &url,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "profile",
				Value:       "0",
				Usage:       "Send HTTP GET request to my webstie `N` times and time the requests",
				Destination: &times,
			},
		},
	}

	app.Action = getRes
	app.Name = "getURL"
	app.UsageText = "Send HTTP GET request to the provided URL"

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func getRes(c *cli.Context) error {
	var host string
	var path string
	var ishttps = true

	t := c.String("profile")
	connect := strings.ToLower(c.String("url"))
	if strings.HasPrefix(connect, "http://") {
		connect = strings.TrimPrefix(connect, "http://")
		ishttps = false
	} else if strings.HasPrefix(connect, "https://") {
		connect = strings.TrimPrefix(connect, "https://")
	}
	url := strings.SplitN(connect, "/", 2)
	if len(url) == 2 {
		host, path = url[0], "/"+url[1]
	} else {
		host, path = url[0], "/"
	}

	if t == "0" {
		requestTo(ishttps, host, path, nil)
	} else {
		t, err := strconv.Atoi(t)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		getProfile(t, host, path, ishttps)
	}
	return nil
}

func requestTo(ishttps bool, host string, path string, channel chan Profile) {
	var tlsConn *tls.Conn
	var netConn net.Conn
	var err error
	var reader *bufio.Reader
	start := time.Now()

	if ishttps {
		tlsConn, err = tls.Dial("tcp", host+":443", nil)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Fprintf(tlsConn, method+path+protocol+header+host+tail)
		reader = bufio.NewReader(tlsConn)
	} else {
		netConn, err = net.Dial("tcp", host+":80")
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Fprintf(netConn, method+path+protocol+header+host+tail)
		reader = bufio.NewReader(netConn)
	}

	var output string
	var totalLen int64
	var status int
	var cLen = int64(-1)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		if len(output) == 0 {
			status, err = strconv.Atoi(line[9:12])
			if err != nil {
				fmt.Println(err)
				return
			}
		}
		output += line

		if line == "\r\n" {
			break
		}
		if strings.Contains(line, "Content-Length: ") {
			clVal := strings.Replace(strings.TrimRight(line, "\r\n"), "Content-Length: ", "", 1)
			clValInt, err := strconv.Atoi(clVal)

			if err != nil {
				fmt.Println(err)
				return
			}
			cLen = int64(clValInt)
		}
	}

	if cLen > 0 {
		sb := new(strings.Builder)
		io.CopyN(sb, reader, cLen)
		output += sb.String()
	} else if cLen == -1 {
		for {
			line, _ := reader.ReadString('\n') // read the chunk size in hex
			cSize, err := strconv.ParseInt(strings.TrimRight(line, "\r\n"), 16, 64)
			if err != nil {
				fmt.Println(err)
				return
			}
			totalLen += cSize
			if cSize == 0 {
				break
			}

			sb := new(strings.Builder)
			io.CopyN(sb, reader, cSize) // read the chunk
			output += sb.String()

			reader.ReadString('\n') // read & dump the \r\n in the end of the chunk
		}
	}

	if channel == nil {
		fmt.Print(output)
	} else {
		elapsed := time.Since(start).Milliseconds()
		channel <- Profile{status, elapsed, len(output)}
	}

	if ishttps {
		tlsConn.Close()
	} else {
		netConn.Close()
	}

	return
}

func getProfile(t int, host string, path string, ishttps bool) {
	if t <= 0 {
		fmt.Println("Please enter an positive integer.")
		return
	}

	channel := make(chan Profile)
	times := make([]int64, t)
	var errorCodes []int
	var errors string
	var smallest int
	var largest int
	var fastest int64
	var slowest int64

	for i := 0; i < t; i++ {
		go requestTo(ishttps, host, path, channel)
	}

	for i := 0; i < t; i++ {
		prof := <-channel

		times[i] = prof.time
		if fastest == 0 || prof.time < fastest {
			fastest = prof.time
		}
		if prof.time > slowest {
			slowest = prof.time
		}
		if prof.code >= 400 {
			errorCodes = append(errorCodes, prof.code)
		}
		if smallest == 0 || prof.size < smallest {
			smallest = prof.size
		}
		if prof.size > largest {
			largest = prof.size
		}
	}

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	if len(errorCodes) == 0 {
		errors = "None"
	} else {
		var tmp []string
		for i := range errorCodes {
			code := strconv.Itoa(errorCodes[i])
			tmp = append(tmp, code)
		}
		errors = strings.Join(tmp, ", ")
	}

	fmt.Println("The number of requests:", t)
	fmt.Println("The fastest time:", fastest, "Milliseconds")
	fmt.Println("The slowest time:", slowest, "Milliseconds")
	fmt.Println("The mean & median times:", mean(times), "Milliseconds,", median(times), "Milliseconds")
	fmt.Println("The percentage of requests that succeeded:", math.Round(float64(t-len(errorCodes))/float64(t)*100), "%")
	fmt.Println("Any error codes returned that weren't a success:", errors)
	fmt.Println("The size in bytes of the smallest response:", smallest, "bytes")
	fmt.Println("The size in bytes of the largest response:", largest, "bytes")

	return
}

func mean(arr []int64) float64 {
	var ans int64
	for i := 0; i < len(arr); i++ {
		ans += arr[i]
	}
	return math.Round(float64(ans) / float64(len(arr)))
}

func median(arr []int64) float64 {
	l := len(arr)
	if l == 1 {
		return float64(arr[0])
	}
	if l%2 == 1 {
		return float64(arr[l/2])
	}
	return math.Round(float64(arr[l/2]+arr[l/2-1]) / float64(2))
}
