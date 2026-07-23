package systemusers

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type User struct { Username string `json:"username"`; UID int `json:"uid"`; GID int `json:"gid"`; Home string `json:"home"`; Shell string `json:"shell"` }
func List() ([]User, error) {
	f, err := os.Open("/etc/passwd"); if err != nil { return nil, err }; defer f.Close()
	var out []User; s := bufio.NewScanner(f)
	for s.Scan() { p:=strings.Split(s.Text(), ":"); if len(p)!=7 {continue}; uid,_:=strconv.Atoi(p[2]); gid,_:=strconv.Atoi(p[3]); out=append(out,User{p[0],uid,gid,p[5],p[6]}) }
	return out, s.Err()
}
