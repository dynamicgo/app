package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	extend "github.com/dynamicgo/go-config-extend"
	"github.com/dynamicgo/go-config/source/envvar"
	"github.com/dynamicgo/go-config/source/file"

	config "github.com/dynamicgo/go-config"
	"github.com/dynamicgo/go-config/source"
	flagsource "github.com/dynamicgo/go-config/source/flag"
	"github.com/dynamicgo/slf4go"
)

// ServiceMain .
type ServiceMain func(config config.Config) error

type serviceRegister struct {
	slf4go.Logger
	sync.RWMutex
	services map[string]ServiceMain
}

var globalServiceRegisterOnce sync.Once
var globalServiceRegister *serviceRegister

func initServiceRegister() {
	globalServiceRegister = &serviceRegister{
		Logger:   slf4go.Get("service-register"),
		services: make(map[string]ServiceMain),
	}
}

// ImportService .
func ImportService(name string, main ServiceMain) {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	_, ok := globalServiceRegister.services[name]

	if ok {
		panic(fmt.Sprintf("duplicate import service %s", name))
	}

	globalServiceRegister.services[name] = main

	globalServiceRegister.InfoF("import service %s", name)
}

func getImportServices() map[string]ServiceMain {
	globalServiceRegisterOnce.Do(initServiceRegister)

	globalServiceRegister.Lock()
	defer globalServiceRegister.Unlock()

	services := make(map[string]ServiceMain)

	for name, s := range globalServiceRegister.services {
		services[name] = s
	}
	return services
}

var logger slf4go.Logger

// Run .
func Run(tag string) {
	tag = strings.ToUpper(tag)
	logger = slf4go.Get(tag)

	defer func() {
		time.Sleep(time.Second * 2)

		println("mesh app exit")
	}()

	configpath := flag.String("config", "", "special the mesh app config file")

	flag.Parse()

	config, err := createConfig(*configpath)

	if err != nil {
		logger.ErrorF("create config error %s", err)
		return
	}

	logconfig, err := extend.SubConfig(config, "slf4go")

	if err != nil {
		logger.Info(fmt.Sprintf("get slf4go config error: %s", err))
		return
	}

	if err := slf4go.Load(logconfig); err != nil {
		logger.Info(fmt.Sprintf("load slf4go config error: %s", err))
		return
	}

	services := getImportServices()

	if len(services) == 0 {
		logger.Info(fmt.Sprintf("[%s] run nothing, exit", tag))
		return
	}

	var wg sync.WaitGroup

	for name, f := range getImportServices() {
		wg.Add(1)
		go runService(&wg, config, name, f)
	}

	wg.Wait()
}

func runService(wg *sync.WaitGroup, config config.Config, name string, f ServiceMain) {
	defer wg.Done()

	logger.Info(fmt.Sprintf("service %s running...", name))

	if err := f(config); err != nil {
		logger.Info(fmt.Sprintf("service %s stop with err: %s", name, err))
	}

	logger.InfoF("service %s stopped", name)
}

func createConfig(configpath string) (config.Config, error) {
	configs, err := loadconfigs(configpath)

	if err != nil {
		return nil, err
	}

	config := config.NewConfig()

	sources := []source.Source{
		envvar.NewSource(envvar.WithPrefix()),
		flagsource.NewSource(),
	}

	for _, path := range configs {
		sources = append(sources, file.NewSource(file.WithPath(path)))
	}

	err = config.Load(sources...)

	return config, err
}

func loadconfigs(path string) ([]string, error) {
	fi, err := os.Stat(path)

	if err != nil {
		return nil, err
	}

	if !fi.IsDir() {
		return []string{
			path,
		}, nil
	}

	var files []string

	err = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if path == "." || path == ".." {
			return err
		}

		files = append(files, path)
		return err
	})

	if err != nil {
		return nil, err
	}

	return files, err
}
