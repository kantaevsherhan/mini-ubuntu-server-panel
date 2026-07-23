package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/config"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/httpapi"
)

var version = "dev"
//go:embed web/*
var web embed.FS

func main() {
	configPath:=flag.String("config","/etc/mini-ubuntu-server/config.yml","configuration file")
	showVersion:=flag.Bool("version",false,"print version"); flag.Parse()
	if *showVersion {fmt.Println(version);return}
	cfg,err:=config.Load(*configPath);if err!=nil{log.Fatal(err)}
	db,err:=database.Open(filepath.Join(cfg.DataDir,"mini-ubuntu-server.db"));if err!=nil{log.Fatal(err)};defer db.Close()
	bootstrap(db)
	app:=fiber.New(fiber.Config{AppName:"Mini Ubuntu Server Panel",ReadTimeout:15*time.Second,WriteTimeout:30*time.Second,ErrorHandler:func(c *fiber.Ctx,e error)error{code:=500;if fe,ok:=e.(*fiber.Error);ok{code=fe.Code};return c.Status(code).JSON(fiber.Map{"error":e.Error()})}})
	app.Use(recover.New(),compress.New()); httpapi.API{DB:db,Secret:cfg.JWTSecret,Version:version}.Register(app)
	root,_:=fs.Sub(web,"web");app.Use("/",filesystem(root));log.Printf("listening on %s",cfg.Listen);log.Fatal(app.Listen(cfg.Listen))
}
func filesystem(root fs.FS) fiber.Handler{return func(c *fiber.Ctx)error{path:=strings.TrimPrefix(c.Path(),"/");if path==""{path="index.html"};b,e:=fs.ReadFile(root,path);if e!=nil{b,e=fs.ReadFile(root,"index.html")};if e!=nil{return fiber.ErrNotFound};if strings.HasSuffix(path,".js"){c.Type("js")}else if strings.HasSuffix(path,".css"){c.Type("css")}else{c.Type("html")};return c.Send(b)}}
func bootstrap(db *sql.DB){
	var n int; if err:=db.QueryRow(`SELECT count(*) FROM users`).Scan(&n);err!=nil||n>0{return}
	username:=os.Getenv("MINI_UBUNTU_SERVER_BOOTSTRAP_USERNAME");password:=os.Getenv("MINI_UBUNTU_SERVER_BOOTSTRAP_PASSWORD")
	if username==""||password==""{log.Print("no users exist; set bootstrap environment variables");return}
	h,err:=auth.Hash(password);if err!=nil{log.Fatal(err)};now:=time.Now().UTC()
	_,err=db.Exec(`INSERT INTO users(username,display_name,password_hash,role,is_active,must_change_password,created_at,updated_at) VALUES(?,?,?,'admin',1,1,?,?)`,username,"Administrator",h,now,now)
	if err!=nil{log.Fatal(err)};log.Printf("bootstrap administrator %q created; password is not logged",username)
}
