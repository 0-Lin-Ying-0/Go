package infrastructure

import (
	"bufio"
	"encoding/csv"
	"os"
	"strings"
)

type FileHandler struct{}

func NewFileHandler() *FileHandler {
	return &FileHandler{}
}

// ReadLines 读取输入文件
func (f *FileHandler) ReadLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// WriteRecord 写入CSV一行
func (f *FileHandler) WriteRecord(filename string, record []string) error {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 写入 UTF-8 BOM 头 (可选，防止 Excel 打开乱码)
	// file.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(file)
	defer writer.Flush()
	return writer.Write(record)
}
