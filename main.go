package main

import (
	"crypto"
	_ "crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"code.google.com/p/go.exp/inotify"
)

const flags = inotify.IN_MOVED_TO | inotify.IN_CLOSE_WRITE

var (
	postURL   string
	patinc    string
	keepFiles bool
	noscan    bool
	verbose   bool
)

func main() {
	flag.StringVar(&patinc, "i", "*", "include pattern")
	flag.BoolVar(&keepFiles, "k", false, "keep files")
	flag.BoolVar(&noscan, "ns", false, "no initial file scan")
	flag.BoolVar(&verbose, "v", false, "verbose logging")
	flag.Parse()
	args := flag.Args()
	if !verbose {
		LogLvl = LvlError
	}

	_, err := filepath.Match(patinc, "")
	if err != nil {
		log.Fatalln("Invalid include pattern")
	}

	switch len(args) {
	case 0:
		log.Fatalln("usage:", os.Args[0], "post-url [watch-dirs...]")
	case 1:
		args = append(args, ".")
	}
	postURL, args = args[0], args[1:]
	u, err := url.Parse(postURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		log.Fatalln("post-url must be a valid http(s) url", postURL)
	}

	wat, err := inotify.NewWatcher()
	if err != nil {
		log.Fatalln("Error creating watcher:", err)
	}
	for _, arg := range args {
		err := wat.AddWatch(arg, flags)
		if err != nil {
			log.Fatalln("Error creating watch:", err)
		} else if !noscan {
			go scan(arg)
		}
	}

	for {
		select {
		case ev := <-wat.Event:
			go handle(ev.Name)
		case err := <-wat.Error:
			log.Fatalln("Error:", err)
		}
	}
}

func scan(dir string) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatalln("Unable to scan %q: %v", dir, err)
	}
	for _, info := range infos {
		handle(filepath.Join(dir, info.Name()))
	}
}

func handle(path string) {
	path = filepath.Clean(path)
	if match(path) {
		send(path)
	}
}

func match(path string) bool {
	base := filepath.Base(path)
	if base[0] == '.' {
		return false
	}
	ok, err := filepath.Match(patinc, base)
	if err != nil {
		panic("impossible: pattern no longer valid?")
	} else if !ok {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		Log(LvlError, "Unable to stat %q: %v", path, err)
	} else if !info.IsDir() {
		return true
	}
	return false
}

func send(path string) {
	Log(LvlDebug, "Handling %q", path)
	f, err := os.Open(path)
	if err != nil {
		Log(LvlError, "Unable to open %q: %v", path, err)
		return
	}
	defer f.Close()
	r, w := io.Pipe()
	mp := multipart.NewWriter(w)
	contentType := mp.FormDataContentType()
	go func() {
		h := crypto.SHA1.New()
		hr := io.TeeReader(f, h)
		fw, err := mp.CreateFormFile("file", filepath.Base(path))
		if err != nil {
			Log(LvlError, "error creating multipart attachment: %v", err)
			goto fail
		}
		_, err = io.Copy(fw, hr)
		if err != nil {
			Log(LvlError, "error serializing file: %v", err)
			goto fail
		}
		mp.WriteField("path", path)
		mp.WriteField("sha1", hex.EncodeToString(h.Sum(nil)))
		mp.Close()
		w.Close()
		return
	fail:
		w.CloseWithError(fmt.Errorf("request body construction failed"))
	}()
	resp, err := http.Post(postURL, contentType, r)
	if err != nil {
		Log(LvlError, "Request failed: %v", err)
	} else if resp.StatusCode >= 400 {
		buf := make([]byte, 1024)
		n, _ := io.ReadFull(resp.Body, buf)
		Log(LvlError, "Server indicated failure: %s; %s", resp.Status, buf[:n])
	} else if keepFiles {
		return
	} else if err := os.Remove(path); err != nil {
		Log(LvlError, "Failed to remove %q: %v", path, err)
	} else {
		Log(LvlInfo, "Removed %q", path)
	}
}
