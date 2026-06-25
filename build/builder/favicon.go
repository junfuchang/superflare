package builder

import (
	"fmt"
	"log"
	"os"
)

func TaskForFavicon(src string, dest string) {
	if stat, err := os.Stat(src); err == nil && stat.IsDir() {
		_PrepareDirectory(dest)
		if err := _CopyDirectoryWithoutSymlink(src, dest); err != nil {
			log.Fatal(err)
		}
		fmt.Println("复制站点图标目录 ... [OK]")
		return
	}

	if err := _Copy(src, dest); err != nil {
		log.Fatal(err)
	}
	fmt.Println("复制站点图标文件 ... [OK]")
}
