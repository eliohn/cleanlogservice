package main

import (
	"fmt"
	"github.com/kardianos/service"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/robfig/cron/v3"
)

type Config struct {
	Directories []string `yaml:"directories"`
	Days        int      `yaml:"days"`
	Time        string   `yaml:"time"`
}

type program struct {
	exit    chan struct{}
	logger  *log.Logger
	config  Config
	logFile *lumberjack.Logger
}

func (p *program) Start(s service.Service) error {
	p.logger.Printf("Service started")
	go p.cleanDirectories()
	go p.run()
	return nil
}

func (p *program) run() {
	c := cron.New(cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		cron.WithLogger(
			cron.VerbosePrintfLogger(
				log.New(p.logFile, "", log.LstdFlags),
			),
		),
	)
	_, err := c.AddFunc(p.config.Time, p.cleanDirectories)
	if err != nil {
		return
	}
	c.Start()

	<-p.exit
	c.Stop()

	p.logger.Printf("Service stopped")
}

func (p *program) Stop(s service.Service) error {
	close(p.exit)
	return nil
}

func (p *program) loadConfig(configFilePath string) (Config, error) {
	var config Config
	executable, err := os.Executable()
	p.logger.Printf("当前文件夹路径：" + executable)
	if err != nil {
		return Config{}, err
	}
	if configFilePath != "" {
		viper.SetConfigFile(configFilePath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(getCurrentAbPathByExecutable())

	}

	err = viper.ReadInConfig()
	if err != nil {
		return config, err
	}

	viper.SetDefault("days", 3)

	err = viper.Unmarshal(&config)
	if err != nil {
		return config, err
	}
	p.logger.Printf("配置信息读取结果如下：")
	p.logger.Printf("Time:" + config.Time)
	p.logger.Printf("Days:", config.Days)
	p.logger.Printf("Directories:", config.Directories)

	return config, nil
}

// 获取当前执行程序所在的绝对路径
func getCurrentAbPathByExecutable() string {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	res, _ := filepath.EvalSymlinks(filepath.Dir(exePath))
	return res
}

func main() {
	sArgs := fmt.Sprint(os.Args)

	// 创建一个新的程序实例
	prg := &program{
		exit: make(chan struct{}),
	}

	// 打开日志文件
	logFileName := "cleanlog.log"
	logFilePath := filepath.Join(getCurrentAbPathByExecutable(), "logs", logFileName)
	logFile := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10, // 每个日志文件最大10MB
		MaxBackups: 5,  // 最多保留3个旧日志文件
		MaxAge:     10, // 保留最近30天的日志文件
		Compress:   false,
		LocalTime:  true,
	}
	prg.logFile = logFile
	prg.logger = log.New(logFile, "", log.LstdFlags)
	prg.logger.Printf("开始执行")
	prg.logger.Printf("Args:" + sArgs)

	// 创建一个新的服务
	svcConfig := &service.Config{
		Name:        "A乐榜日志清理服务",
		DisplayName: "A乐榜日志清理服务",
		Description: "乐榜日志清理服务，配置在文件同目录下的config.yml"}
	// 创建一个新的服务对象
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	prg.logger.Printf("服务创建！")
	// 检查命令行参数
	if len(os.Args) > 1 {
		prg.logger.Printf("有参数：" + os.Args[1])
		err := service.Control(s, os.Args[1])
		if err != nil {
			log.Fatalf("Failed to %s service: %s", os.Args[1], err)
		}
		return
	}
	// 从命令行参数获取配置文件路径
	configFilePath := "" // 在这里设置默认的配置文件路径
	if len(os.Args) > 2 {
		configFilePath = os.Args[2]
	}
	prg.logger.Printf("开始加载配置！")
	// 从文件加载配置
	config, err := prg.loadConfig(configFilePath)
	if err != nil {
		log.Fatalf("加载配置文件时发生错误: %s", err)
	}
	prg.config = config
	prg.logger.Printf("配置加载完成！")
	// 检查服务是否已经在运行
	status, err := s.Status()
	if err == nil {
		prg.logger.Printf("Service is already %s", status)
	}

	// 启动服务
	err = s.Run()
	if err != nil {
		prg.logger.Fatal(err)
	}

	select {}
}

func (p *program) cleanDirectories() {
	//now := time.Now()
	p.logger.Printf("---------------   执行一次任务！ ---------------")
	successCount := 0
	failureCount := 0
	threshold := time.Now().AddDate(0, 0, -p.config.Days).Unix()
	//threshold := time.Now().AddDate(0, 0, -p.config.Days).Unix()
	for _, dir := range p.config.Directories {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, file := range files {
			filePath := filepath.Join(dir, file.Name())
			info, err := file.Info()
			if err != nil {
				fmt.Println("获取文件信息失败:", err)
				failureCount++
				continue // 获取文件信息失败，跳过当前文件，继续下一个文件
			}

			if !file.IsDir() && info.ModTime().Unix() < threshold {
				err := os.Remove(filePath)
				if err != nil {
					p.logger.Println("删除文件失败:", err)
					failureCount++
					continue // 删除失败，跳过当前文件，继续下一个文件
				}
				//fmt.Println("删除文件成功:", filePath)
				successCount++
			}
		}
	}

	p.logger.Printf("成功删除文件数: %d\n", successCount)
	p.logger.Printf("删除文件失败数: %d\n", failureCount)
}
