package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
)

const (
	exitCodeDifferentDimension = 65
	exitCodeDifferentPixels    = 66
)

var (
	threshold        string
	srcDir           string
	targetDir        string
	defaultThreshold = "0.015"
	defaultSrcDir    = "src"
	defaultTargetDir = "target"
	tmpDir           = "tmp"
	container        = "arisawa/pixelmatch:v5.1.0"
)

type diffPixel struct {
	file   string
	pixels string
	error  string
}

func newDiffPixel(fileName, pixelmatchOut string) *diffPixel {
	// fmt.Println(pixelmatchOut)
	lines := strings.Split(pixelmatchOut, "\n")
	pixels := strings.Split(lines[1], ":")
	errorPer := strings.Split(lines[2], ":")
	return &diffPixel{
		file: fileName,
		pixels: strings.TrimSpace(pixels[1]),
		error: strings.TrimSpace(errorPer[1]),
	}
}

func main() {
	flag.Usage = func() {
		fmt.Printf(`Usage:
  %s THRESHOLD SRC_DIR TARGET_DIR

  Compare png files in the source directory with the same name of file in the target directory by pixelmatch docker container.
    THRESHOLD string
      threshold for pixelmatch 0 (default "%s", range is 0 to 1)
    SRC_DIR string
      source directory (default "%s")
    TARGET_DIR string
      target directory (default "%s")

`, os.Args[0], defaultThreshold, defaultSrcDir, defaultTargetDir)
		flag.PrintDefaults()
	}
	flag.Parse()

	if threshold = flag.Arg(0); threshold == "" {
		threshold = defaultThreshold
	}
	if srcDir = flag.Arg(1); srcDir == "" {
		srcDir = defaultSrcDir
	}
	if targetDir = flag.Arg(1); targetDir == "" {
		targetDir = defaultTargetDir
	}

	validate()
	checkTmpDir()

	diffDimensions := []string{}
	diffPixels := []*diffPixel{}

	files, err := ioutil.ReadDir(srcDir)
	if err != nil {
		log.Fatalf("read %s error: %s", srcDir, err)
	}
	for _, f := range files {
		fileName := f.Name()
		if !strings.HasSuffix(fileName, ".png") {
			continue
		}

		srcFile := filepath.Join(srcDir, fileName)
		targetFile := filepath.Join(targetDir, fileName)

		if _, err := os.Stat(targetFile); os.IsNotExist(err) {
			continue
		}

		tmpSrcFile := filepath.Join(tmpDir, fmt.Sprintf("src-%s", fileName))
		tmpTargetFile := filepath.Join(tmpDir, fmt.Sprintf("target-%s", fileName))

		copyFile(srcFile, tmpSrcFile)
		copyFile(targetFile, tmpTargetFile)

		diffFileName := fmt.Sprintf("diff-%s", fileName)
		diffFile := filepath.Join(tmpDir, diffFileName)

		fmt.Printf("check %s\n", srcFile)

		absTmpDir, err := filepath.Abs(tmpDir)
		if err != nil {
			log.Fatalf("cannot get absolute path: %s", tmpDir)
		}
		volume := fmt.Sprintf("%s:/app/%s", absTmpDir, tmpDir)
		cmd := exec.Command(
			"docker", "run", "--rm", "-v", volume, container,
			tmpSrcFile, tmpTargetFile, diffFile, threshold,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			switch cmd.ProcessState.ExitCode() {
			case exitCodeDifferentDimension:
				diffDimensions = append(diffDimensions, fileName)
			case exitCodeDifferentPixels:
				diffPixels = append(diffPixels, newDiffPixel(fileName, string(out)))
				if err := os.Rename(diffFile, diffFileName); err != nil {
					if err != nil {
						log.Fatalf("file move error: %v", err)
					}
				}
			default:
				log.Fatalf("command execution error: %v", err)
			}
		} else {
			if err := os.Remove(diffFile); err != nil {
				if err != nil {
					log.Fatalf("file remove error: %v", err)
				}
			}
		}
		if err := os.Remove(tmpSrcFile); err != nil {
			log.Fatalf("tmp source file remove error: %v", err)
		}
		if err := os.Remove(tmpTargetFile); err != nil {
			log.Fatalf("tmp source file remove error: %v", err)
		}
	}

	if len(diffDimensions) > 0 {
		fmt.Println("-- dimensions do not match --")
		for _, f := range diffDimensions {
			fmt.Println(f)
		}
	}

	if len(diffPixels) > 0 {
		fmt.Println("-- Different pixels are found --")
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 0, '\t', 0)
		for _, dp := range diffPixels {
			fmt.Fprintf(w, "%s\t%s\t%s\n", dp.file, dp.pixels, dp.error)
		}
		w.Flush()
	}
}

func validate() {
	if _, err := exec.LookPath("docker"); err != nil {
		log.Fatal("docker is not installed")
	}

	if _, err := strconv.ParseFloat(threshold, 32); err != nil {
		log.Fatalf("threshold error: %v", err)
	}
	stat, err := os.Stat(srcDir)
	if err != nil {
		log.Fatalf("source directory error: %v", err)
	}
	if !stat.IsDir() {
		log.Fatalf("source: %s is not directory", srcDir)
	}

	stat, err = os.Stat(targetDir)
	if err != nil {
		log.Fatalf("target directory error: %v", err)
	}
	if !stat.IsDir() {
		log.Fatalf("target: %s is not directory", targetDir)
	}
}

func checkTmpDir() {
	stat, err := os.Stat(tmpDir)
	if os.IsNotExist(err) {
		if err = os.Mkdir(tmpDir, 0755); err != nil {
			log.Fatalf("tmp directory error: %v", err)
		}
		return
	}
	if !stat.IsDir() {
		log.Fatalf("%s is not directory", tmpDir)
	}
}

func copyFile(src, dst string) {
	b, err := ioutil.ReadFile(src)
	if err != nil {
		log.Fatalf("read file error: %v", err)
	}

	if err := ioutil.WriteFile(dst, b, 0644); err != nil {
		log.Fatalf("copy file error: %v", err)
	}
}
