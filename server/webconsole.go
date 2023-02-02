package server

import (
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/emersion/go-webdav"
	gorilla "github.com/gorilla/websocket"
	"github.com/lcpu-dev/vmsched/utils"
	lxd "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"github.com/pkg/sftp"
	"nhooyr.io/websocket"
)

func (s *Server) HandleSpiceWs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("token_name")
	secret := r.URL.Query().Get("token_secret")
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
	}
	u := s.credentialToUser(name, secret)
	if !s.userHaveAccessTo(u, "user", "", instance, "") {
		w.WriteHeader(403)
		w.Write([]byte("Bad Request"))
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary", "base64"},
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
		return
	}
	chDisconnect := make(chan bool)
	op, err := s.lxd.ConsoleInstance(instance, api.InstanceConsolePost{
		Type: "vga",
	}, &lxd.InstanceConsoleArgs{
		Terminal: utils.WebSocketConnToConn(
			conn,
			func() error {
				chDisconnect <- true
				return nil
			},
		),
		Control:           func(conn *gorilla.Conn) {},
		ConsoleDisconnect: chDisconnect,
	})
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = op.Wait()
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
	}
}

func (s *Server) SendTermSize(control *gorilla.Conn, width int, height int) error {
	msg := api.InstanceExecControl{}
	msg.Command = "window-resize"
	msg.Args = make(map[string]string)
	msg.Args["width"] = strconv.Itoa(width)
	msg.Args["height"] = strconv.Itoa(height)

	return control.WriteJSON(msg)
}

func (s *Server) HandleConsoleWs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("token_name")
	secret := r.URL.Query().Get("token_secret")
	width, err := strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil {
		width = 80
	}
	height, err := strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil {
		height = 25
	}
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
	}
	u := s.credentialToUser(name, secret)
	if !s.userHaveAccessTo(u, "user", "", instance, "") {
		w.WriteHeader(403)
		w.Write([]byte("Bad Request"))
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary", "base64"},
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
		return
	}
	chDisconnect := make(chan bool)
	op, err := s.lxd.ConsoleInstance(instance, api.InstanceConsolePost{
		Width:  width,
		Height: height,
		Type:   "console",
	}, &lxd.InstanceConsoleArgs{
		Terminal: utils.WebSocketConnToConn(
			conn,
			func() error {
				chDisconnect <- true
				return nil
			},
		),
		Control: func(conn *gorilla.Conn) {
			// TODO: handle resize
		},
		ConsoleDisconnect: chDisconnect,
	})
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = op.Wait()
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
	}
}

func (s *Server) HandleExecWs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("token_name")
	secret := r.URL.Query().Get("token_secret")
	width, err := strconv.Atoi(r.URL.Query().Get("width"))
	if err != nil {
		width = 80
	}
	height, err := strconv.Atoi(r.URL.Query().Get("height"))
	if err != nil {
		height = 25
	}
	cmd := r.URL.Query().Get("cmd")
	instance := r.URL.Query().Get("instance")
	if instance == "" {
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
	}
	u := s.credentialToUser(name, secret)
	if !s.userHaveAccessTo(u, "user", "", instance, "") {
		w.WriteHeader(403)
		w.Write([]byte("Bad Request"))
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       []string{"binary", "base64"},
	})
	if err != nil {
		log.Println(err)
		w.WriteHeader(400)
		w.Write([]byte("Bad Request"))
		return
	}
	chDisconnect := make(chan bool)
	wr := utils.WebSocketConnToConn(
		conn,
		func() error {
			chDisconnect <- true
			return nil
		},
	)
	op, err := s.lxd.ExecInstance(instance, api.InstanceExecPost{
		Command:     []string{"bash", "-c", "TERM=screen " + cmd},
		WaitForWS:   true,
		Interactive: true,
		Width:       width,
		Height:      height,
		User:        0,
		Group:       0,
	}, &lxd.InstanceExecArgs{
		Stdin:  wr,
		Stdout: wr,
		Stderr: wr,
		Control: func(conn *gorilla.Conn) {
			// TODO: handle resize
			<-chDisconnect
			conn.WriteMessage(gorilla.CloseMessage, gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, "bye"))
		},
	})
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
		return
	}
	err = op.Wait()
	if err != nil {
		log.Println(err)
		conn.Close(websocket.StatusAbnormalClosure, err.Error())
	}
}

type SftpFs struct {
	*sftp.Client
	pathPrefix string
}

func (fs *SftpFs) stripPath(path string) string {
	i := strings.Index(path, fs.pathPrefix)
	if i == -1 {
		return path
	}
	return path[i+len(fs.pathPrefix):]
}

func (fss *SftpFs) Copy(name, dest string, recursive, overwrite bool) (created bool, err error) {
	dest = fss.stripPath(dest)
	var stat fs.FileInfo
	isDir := false
	if stat, err = fss.Client.Stat(dest); err != nil {
		if !os.IsNotExist(err) {
			log.Println("ERROR stat:", err)
			return false, err
		}
		if recursive {
			st2, err := fss.Client.Stat(name)
			if err != nil {
				return false, err
			}
			if st2.IsDir() {
				err = fss.Client.MkdirAll(dest)
				isDir = true
				if err != nil {
					return false, err
				}
			}
		}
		created = true
	} else {
		isDir = stat.IsDir()
		if !overwrite {
			return false, os.ErrExist
		}
		if err := fss.RemoveAll(dest); err != nil {
			log.Println("ERROR removeAll:", err)
			return false, err
		}
	}

	walker := fss.Client.Walk(name)
	for walker.Step() {
		if walker.Err() != nil {
			log.Println("ERROR walk:", walker.Err())
			return false, walker.Err()
		}

		dst := ""
		if isDir {
			dst = path.Join(dest, strings.TrimPrefix(walker.Path(), name))
		} else {
			dst = dest
		}
		// fmt.Println(walker.Path(), dst)
		if walker.Stat().IsDir() {
			if err := fss.Mkdir(dst); err != nil {
				log.Println("ERROR mkdir:", err)
				return false, err
			}
		} else {
			orig, err := fss.Client.Open(walker.Path())
			if err != nil {
				log.Println("ERROR open:", err)
				return false, err
			}
			tgt, err := fss.Client.Create(dst)
			if err != nil {
				log.Println("ERROR create:", err)
				return false, err
			}
			_, err = io.Copy(tgt, orig)
			if err != nil {
				log.Println("ERROR copy:", err)
				return false, err
			}
			err = orig.Close()
			if err != nil {
				log.Println("ERROR close:", err)
				return false, err
			}
			err = tgt.Close()
			if err != nil {
				log.Println("ERROR close:", err)
				return false, err
			}
		}

		if walker.Stat().IsDir() && !recursive {
			walker.SkipDir()
		}
	}

	return created, nil
}

func (fs *SftpFs) Create(name string) (io.WriteCloser, error) {
	f, e := fs.Client.Create(name)
	return f, e
}

func (fs *SftpFs) Open(name string) (io.ReadCloser, error) {
	return fs.Client.Open(name)
}

func (fs *SftpFs) MoveAll(name, dest string, overwrite bool) (created bool, err error) {
	created, err = fs.Copy(name, dest, true, overwrite)
	if err != nil {
		log.Println("ERROR:", err)
		return
	}
	err = fs.RemoveAll(name)
	return
}

func (fs *SftpFs) checkDir(dir string, base string, rslt *[]webdav.FileInfo, recursive bool) error {
	dirs, err := fs.Client.ReadDir(dir)
	if err != nil {
		log.Println("ERROR:", err)
		return err
	}
	for _, dir := range dirs {
		*rslt = append(*rslt, webdav.FileInfo{
			Path:     path.Join(base, dir.Name()),
			Size:     dir.Size(),
			ModTime:  dir.ModTime(),
			IsDir:    dir.IsDir(),
			MIMEType: mime.TypeByExtension(path.Ext(dir.Name())),
			ETag:     "",
		})
		if dir.IsDir() && recursive {
			fs.checkDir(path.Join(base, dir.Name()), base, rslt, recursive)
		}
	}
	return nil
}

func (fs *SftpFs) Readdir(name string, recursive bool) ([]webdav.FileInfo, error) {
	var rslt []webdav.FileInfo
	err := fs.checkDir(name, "", &rslt, recursive)
	if err != nil {
		log.Println("ERROR:", err)
		return nil, err
	}
	return rslt, nil
}

func (fs *SftpFs) RemoveAll(name string) error {
	f, err := fs.Client.Stat(name)
	if err != nil {
		return err
	}
	if !f.IsDir() {
		err = fs.Client.Remove(name)
		if err != nil {
			log.Println("ERROR:", err)
			return err
		}
	} else {
		dir, err := fs.Client.ReadDir(name)
		if err != nil {
			log.Println("ERROR:", err)
			return err
		}
		for _, item := range dir {
			err = fs.RemoveAll(path.Join(name, item.Name()))
			if err != nil {
				log.Println("ERROR:", err)
				return err
			}
		}
		err = fs.Client.RemoveDirectory(name)
		if err != nil {
			log.Println("ERROR:", err)
			return err
		}
	}
	return nil
}

func (fs *SftpFs) Stat(name string) (*webdav.FileInfo, error) {
	inf, err := fs.Client.Stat(name)
	if err != nil {
		log.Println("ERROR:", err)
		return nil, err
	}
	return &webdav.FileInfo{
		Path:     inf.Name(),
		Size:     inf.Size(),
		ModTime:  inf.ModTime(),
		IsDir:    inf.IsDir(),
		MIMEType: mime.TypeByExtension(path.Ext(inf.Name())),
		ETag:     "",
	}, nil
}

func (fs *SftpFs) Mkdir(name string) error {
	err := fs.Client.MkdirAll(name)
	if err != nil {
		log.Println("ERROR:", err)
	}
	return err
}

func (s *Server) HandleWebDAV(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	instance := parts[1]
	prefixToStrip := "/" + parts[0] + "/" + instance
	u, p, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	user := s.credentialToUser(u, p)
	if !s.userHaveAccessTo(user, "user", "", instance, "") {
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	client, err := s.lxd.GetInstanceFileSFTP(instance)
	if err != nil {
		log.Println("ERROR:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer client.Close()
	h := http.StripPrefix(prefixToStrip, &webdav.Handler{
		FileSystem: &SftpFs{
			Client:     client,
			pathPrefix: prefixToStrip,
		},
	})
	h.ServeHTTP(w, r)
}
