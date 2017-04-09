// Usage: $0 srcdir dstdir
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	// "github.com/maemual/shutil"
	"github.com/gosexy/exif"
	shutil "github.com/termie/go-shutil"
)

func bincompare(i1, i2 io.Reader, bufsz int) int {
	b1 := make([]byte, bufsz)
	b2 := make([]byte, bufsz)
	for {
		n1, e1 := io.ReadAtLeast(i1, b1, bufsz)
		n2, e2 := io.ReadAtLeast(i2, b2, bufsz)
		clen := n1
		if clen > n2 {
			clen = n2
		}
		r := bytes.Compare(b1, b2)
		if r != 0 {
			return r
		}
		if n1 < n2 {
			return -1
		} else if n1 > n2 {
			return 1
		}
		if e1 != nil && e2 != nil {
			return 0
		}
		if e1 != nil {
			return -1
		}
		if e2 != nil {
			return 1
		}
	}
}

func nextfn(src string) string {
	dirn, base := path.Split(src)
	ext := path.Ext(src)
	log.Println("nextfn: dirn=", dirn, ", base=", base, ", ext=", ext)
	base = strings.TrimSuffix(base, ext)
	log.Println("nextfn: trim=", base)
	nn := strings.Split(base, "_")
	log.Println("nextfn: nn=", nn)
	r := 1
	if len(nn) >= 2 && nn[len(nn)-1] != "" {
		var err error
		if r, err = strconv.Atoi(nn[1]); err == nil {
			r += 1
		}
	}
	log.Println("nextfn: nn2=", nn, ", r=", r, "ext=", ext)
	return path.Join(dirn, fmt.Sprintf("%s_%d%s", nn[0], r, ext))
}

func stampfn(src string) (dst string, ts time.Time, err error) {
	fi, err := os.Stat(src)
	if err != nil {
		return
	}
	ts = fi.ModTime()
	dst = ts.Format("2006/01/02/150405") + path.Ext(src)
	return
}

func getstamp(tbl map[string]string) (out string, ok bool) {
	tags := []string{"Date and Time", "Date and Time (Digitized)", "Date and Time (Original)"}
	for _, k := range tags {
		if out, ok := tbl[k]; ok {
			return out + " " + "+0900", true
		}
	}
	if out, ok := tbl["GPS Date"]; ok {
		if out2, ok2 := tbl["GPS Time (Atomic Clock)"]; ok2 {
			return out + " " + out2 + " " + "+0000", true
		}
	}
	return "", false
}

func exiffn(src string) (dst string, ts time.Time, err error) {
	reader := exif.New()
	if err = reader.Open(src); err != nil {
		log.Println("exif open", err)
		return exiffn_cmd(src)
	}
	getstamp(reader.Tags)
	loc, _ := time.LoadLocation("Asia/Tokyo")
	if stamp, ok := getstamp(reader.Tags); ok {
		if ts, err = time.Parse("2006:01:02 15:04:05 -0700", stamp); err == nil {
			dst = ts.In(loc).Format("2006/01/02/150405") + path.Ext(src)
		} else {
			log.Println("time parse", err)
			return exiffn_cmd(src)
		}
		return
	}
	return
}

func exiffn_cmd(src string) (dst string, ts time.Time, err error) {
	loc := time.Local
	if path.Ext(src) == ".mp4" {
		loc = time.UTC
	}
	var out []byte
	out, err = exec.Command("exiftool", src).Output()
	if err != nil {
		log.Println("exec cmd", err)
		return stampfn(src)
	}
	ids := []string{"Create Date", "Modify Date", "Track Create Date", "Track Modify Date", "Date/Time Original"}
	for _, v := range strings.Split(string(out), "\n") {
		for _, k := range ids {
			if strings.HasPrefix(v, k) {
				l := strings.SplitN(v, ":", 2)
				if len(l) != 2 {
					continue
				}
				if ts, err = time.ParseInLocation("2006:01:02 15:04:05", strings.TrimSpace(l[1]), loc); err == nil {
					dst = ts.Local().Format("2006/01/02/150405") + path.Ext(src)
					return
				}

			}
		}
	}
	log.Println("no match line")
	return stampfn(src)
}

func exifcopy(src, dstbase string) error {
	dstpath, ts, err := exiffn(src)
	log.Println("dstpath", dstpath, "ts", ts, "err", err)
	var dstfull string
	for {
		dstfull = path.Join(dstbase, dstpath)
		if _, err := os.Stat(dstfull); os.IsNotExist(err) {
			// copy
			break
		}
		f1, err := os.Open(src)
		if err != nil {
			return err
		}
		defer f1.Close()
		f2, err := os.Open(dstfull)
		if err != nil {
			return err
		}
		defer f2.Close()
		if bincompare(f1, f2, 8192) == 0 {
			// pass
			log.Println("same content", src, dstfull)
			return nil
		}
		// try next filename
		log.Println("try next", src, dstfull)
		dstpath = nextfn(dstpath)
	}
	// copy file
	log.Println("to mkdir", dstfull)
	err = os.MkdirAll(path.Dir(dstfull), os.ModePerm)
	if err != nil {
		log.Println("mkdir", err)
	}
	_, err = shutil.Copy(src, dstfull, false)
	if err != nil {
		log.Println("copy2", err)
	}
	err = os.Chtimes(dstfull, ts, ts)
	if err != nil {
		log.Println("chtimes", err)
	}
	return err
}

func main() {
	src := os.Args[1]
	dst := os.Args[2]
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if path.Ext(p) == ".thumbnails" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if path.Ext(p) == ".DS_Store" {
			return nil
		}
		exifcopy(p, dst)
		return nil
	})
}
