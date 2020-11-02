package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math"
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
	t := c.String("profile")
	connect := c.String("url")
	url := strings.SplitN(connect, "/", 2)
	var host string
	var path string

	if len(url) == 2 {
		host, path = url[0], "/"+url[1]
	} else {
		host, path = url[0], "/"
	}

	if t == "0" {
		requestTo(host, path, nil)
	} else {
		t, err := strconv.Atoi(t)
		if err != nil {
			fmt.Println(err)
			return nil
		}
		getProfile(t, host, path)
	}
	return nil
}

func requestTo(host string, path string, channel chan Profile) {
	start := time.Now()
	conn, err := tls.Dial("tcp", host+":443", nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Fprintf(conn, method+path+protocol+header+host+tail)
	reader := bufio.NewReader(conn)
	var output string
	var cLen int64
	var totalLen int64
	var status int

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

	if cLen != 0 {
		sb := new(strings.Builder)
		io.CopyN(sb, reader, cLen)
		output += sb.String()
	} else {
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

	conn.Close()
	return
}

func getProfile(t int, host string, path string) {
	if t <= 0 {
		fmt.Println("Please enter an positive integer.")
		return
	}

	channel := make(chan Profile)
	times := make([]int64, t)
	var errorCodes []int
	var smallest int
	var largest int
	var fastest int64
	var slowest int64

	for i := 0; i < t; i++ {
		go requestTo(host, path, channel)
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

	fmt.Println("The number of requests: ", t)
	fmt.Println("The fastest time: ", fastest, "Milliseconds")
	fmt.Println("The slowest time: ", slowest, "Milliseconds")
	fmt.Println("The mean & median times: ", mean(times), "Milliseconds, ", median(times), "Milliseconds")
	fmt.Println("The percentage of requests that succeeded: ", math.Round(float64(t-len(errorCodes))/float64(t)*100), "%")
	fmt.Println("Any error codes returned that weren't a success: ", errorCodes)
	fmt.Println("The size in bytes of the smallest response: ", smallest, "bytes")
	fmt.Println("The size in bytes of the largest response: ", largest, "bytes")

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
