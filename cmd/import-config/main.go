// import-config is an explicit, one-time migration of connection settings only.
package main

import (
	"flag"
	"fmt"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/joho/godotenv"
	"os"
	"path/filepath"
)

func main() {
	from := flag.String("from", "", "legacy .env file")
	flag.Parse()
	if *from == "" {
		fmt.Fprintln(os.Stderr, "请用 -from 指定原项目的 .env")
		os.Exit(1)
	}
	values, err := godotenv.Read(*from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法读取连接配置")
		os.Exit(1)
	}
	root, err := config.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "已有配置，未覆盖。请在应用设置中修改。")
		os.Exit(1)
	}
	c := config.Default()
	c.Model.APIKey = values["HDU_STATION_OPENAI_API_KEY"]
	c.CampusKey = values["HDU_STATION_CAMPUS_KEY"]
	if base := values["HDU_STATION_OPENAI_BASE_URL"]; base != "" {
		c.Model.BaseURL = base
		c.Model.Name = "deepseek-v4.1-flash"
	}
	if c.Model.APIKey == "" {
		fmt.Fprintln(os.Stderr, "原配置没有模型凭证")
		os.Exit(1)
	}
	if err := config.Save(root, c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("连接配置已迁移，模型已设为 DeepSeek V4.1 Flash。")
}
