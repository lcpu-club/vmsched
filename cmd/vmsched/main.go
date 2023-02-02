package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/lcpu-dev/vmsched/models"
	"github.com/lcpu-dev/vmsched/server"
	"github.com/lcpu-dev/vmsched/utils/config"
	_ "github.com/mattn/go-sqlite3"
	"github.com/urfave/cli/v2"
	"xorm.io/xorm"
)

func main() {
	app := &cli.App{}
	app.Name = "vmsched"
	app.Usage = "VM Scheduling Service"
	app.Flags = append(app.Flags, &cli.StringFlag{
		Name:        "configure",
		Aliases:     []string{"config", "c"},
		Usage:       "configure file path",
		Value:       "/etc/vmsched.yml",
		DefaultText: "/etc/vmsched.yml",
	})
	var conf *config.Configure
	app.Before = func(ctx *cli.Context) error {
		var err error
		conf, err = config.LoadConfigure(ctx.String("configure"))
		if err != nil {
			return err
		}
		return nil
	}
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "serve",
		UsageText: "Start the server",
		Action: func(ctx *cli.Context) error {
			srv, err := server.NewServer(conf)
			if err != nil {
				return err
			}
			go srv.StartCron()
			return srv.Run()
		},
	})
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "cron",
		UsageText: "Start the extra cron server",
		Action: func(ctx *cli.Context) error {
			srv, err := server.NewServer(conf)
			if err != nil {
				return err
			}
			srv.StartCron()
			return nil
		},
	})
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "init-db",
		UsageText: "Initialize the database",
		Action: func(ctx *cli.Context) error {
			orm, err := xorm.NewEngine(conf.Database.Driver, conf.Database.DSN)
			if err != nil {
				return err
			}
			err = models.Sync(orm)
			if err != nil {
				return err
			}
			if ok, _ := orm.Exist(&models.User{Role: "admin"}); !ok {
				u := &models.User{
					Name:    "admin",
					Role:    "admin",
					Balance: make(map[string]int),
				}
				_, err = orm.Insert(u)
				if err != nil {
					return err
				}
				fmt.Println("DB initialize finished")
			} else {
				fmt.Println("DB already initialized")
			}
			tok := &models.Token{User: "admin"}
			if ok, _ := orm.Get(tok); !ok {
				tok.Name = "admin_token"
				s := sha256.New()
				s.Write([]byte{byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))})
				s.Write([]byte{byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))})
				s.Write([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
				s.Write([]byte{byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))})
				s.Write([]byte{byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256))})
				tok.Secret = hex.EncodeToString(s.Sum(nil))
				_, err = orm.Insert(tok)
				if err != nil {
					return err
				}
			}
			fmt.Printf("Admin user '%v'\n  token name '%v'\n  secret '%v'\n", tok.User, tok.Name, tok.Secret)
			return nil
		},
	})
	err := app.Run(os.Args)
	if err != nil {
		log.Fatalln(err)
	}
}
