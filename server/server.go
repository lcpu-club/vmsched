package server

import (
	"log"
	"net/http"

	restfulspec "github.com/emicklei/go-restful-openapi/v2"
	"github.com/emicklei/go-restful/v3"
	"github.com/lcpu-dev/vmsched/models"
	"github.com/lcpu-dev/vmsched/utils/config"
	lxd "github.com/lxc/lxd/client"
	"xorm.io/xorm"
)

type Server struct {
	orm  *xorm.Engine
	lxd  lxd.InstanceServer
	conf *config.Configure
}

func NewServer(conf *config.Configure) (*Server, error) {
	s := new(Server)
	s.conf = conf
	orm, err := xorm.NewEngine(conf.Database.Driver, conf.Database.DSN)
	if err != nil {
		return nil, err
	}
	s.orm = orm
	ls, err := lxd.ConnectLXD(conf.LXD.Address, &lxd.ConnectionArgs{
		InsecureSkipVerify: true,
		TLSClientCert:      conf.LXD.ClientCert,
		TLSClientKey:       conf.LXD.ClientKey,
	})
	if err != nil {
		orm.Close()
		return nil, err
	}
	s.lxd = ls
	return s, nil
}

// returns an empty string when auth failure
func (s *Server) credentialToUser(tokenName string, secret string) string {
	t := &models.Token{Name: tokenName}
	ok, err := s.orm.Get(t)
	if err != nil {
		log.Println("ERROR:", err)
		return ""
	}
	if !ok {
		return ""
	}
	if t.Secret != secret {
		return ""
	}
	return t.User
}

func (s *Server) userHaveAccessTo(user string, minRole string, task string, instance string, requiredUser string) bool {
	if user == "" {
		return false
	}
	u := &models.User{Name: user}
	ok, err := s.orm.Get(u)
	if err != nil {
		log.Println("ERROR:", err)
		return false
	}
	if !ok {
		return false
	}
	if minRole != "" {
		if u.Role == "admin" {
			return true
		}
		if minRole == "admin" {
			return false
		}
		if minRole == "user" && u.Role == "banned" {
			return false
		}
	}
	if task != "" {
		t := &models.Task{Name: task}
		ok, err := s.orm.Get(t)
		if err != nil {
			log.Println("ERROR:", err)
			return false
		}
		if !ok {
			return false
		}
		if t.User != u.Name {
			return false
		}
	}
	if instance != "" {
		t := &models.Task{Instance: instance}
		ok, err := s.orm.Get(t)
		if err != nil {
			log.Println("ERROR:", err)
			return false
		}
		if !ok {
			return false
		}
		if t.User != u.Name {
			return false
		}
		if t.Status != "active" {
			return false
		}
	}
	if requiredUser != "" {
		if requiredUser != user {
			return false
		}
	}
	return true
}

func (s *Server) filterAuth(minRole string) restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, fc *restful.FilterChain) {
		ins, ok := req.PathParameters()["instance"]
		if !ok {
			ins = ""
		}
		task, ok := req.PathParameters()["task"]
		if !ok {
			task = ""
		}
		tokenName := req.HeaderParameter("X-Token-Name")
		tokenSecret := req.HeaderParameter("X-Token-Secret")
		if tokenName == "" {
			tokenName = req.QueryParameter("token-name")
			tokenSecret = req.QueryParameter("token-secret")
		}
		user, ok := req.PathParameters()["user"]
		if !ok {
			user = ""
		}
		u := s.credentialToUser(tokenName, tokenSecret)
		req.SetAttribute("user", u)
		if s.userHaveAccessTo(u, minRole, task, ins, user) {
			fc.ProcessFilter(req, resp)
		} else {
			resp.WriteErrorString(403, "Access Denied")
		}
	}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()

	ws := new(restful.WebService)
	ws.Path("/api/v1").Consumes(restful.MIME_JSON).Produces(restful.MIME_JSON)
	ws.Filter(func(r1 *restful.Request, r2 *restful.Response, fc *restful.FilterChain) {
		r2.AddHeader("Access-Control-Allow-Origin", "*")
		fc.ProcessFilter(r1, r2)
	})
	ws.Route(
		ws.PUT("/user").
			Reads(UserPut{}).
			Filter(s.filterAuth("admin")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(500, "Internal Server Error", GeneralResponse{}).
			To(s.PutUser),
	)
	ws.Route(
		ws.GET("/user").
			Filter(s.filterAuth("banned")).
			Returns(200, "OK", UserPut{}).
			Returns(500, "Internal Server Error", nil).
			To(s.GetUser),
	)
	ws.Route(
		ws.GET("/user/{user}").
			Param(restful.PathParameter("user", "username")).
			Filter(s.filterAuth("admin")).
			Returns(200, "OK", UserPut{}).
			Returns(500, "Internal Server Error", nil).
			To(s.GetUser),
	)
	ws.Route(
		ws.PUT("/user/{user}/token").
			Param(restful.PathParameter("user", "username")).
			Reads(TokenPut{}).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "User Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.PutUserToken),
	)
	ws.Route(
		ws.GET("/user/{user}/token").
			Param(restful.PathParameter("user", "username")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", TokenGet{}).
			Returns(404, "User Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetUserToken),
	)
	ws.Route(
		ws.DELETE("/user/{user}/token/{token}").
			Param(restful.PathParameter("user", "username")).
			Param(restful.PathParameter("token", "token name")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "Not Found", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.DeleteUserToken),
	)
	ws.Route(
		ws.GET("/user/{user}/task").
			Param(restful.PathParameter("user", "username")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", []TaskGet{}).
			Returns(404, "User Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetUserTasks),
	)
	ws.Route(
		ws.POST("/user/{user}/task").
			Param(restful.PathParameter("user", "username")).
			Reads(TaskPost{}).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "Not Found", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.PostUserTask),
	)
	ws.Route(
		ws.GET("/task/{task}").
			Param(restful.PathParameter("task", "task name")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", TaskGet{}).
			Returns(404, "Task Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetTask),
	)
	ws.Route(
		ws.DELETE("/task/{task}").
			Param(restful.PathParameter("task", "task name")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "Not Found", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.DeleteTask),
	)
	ws.Route(
		ws.POST("/task/{task}/state").
			Param(restful.PathParameter("task", "task name")).
			Reads(TaskStatePost{}).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "Not Found", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.PostTaskState),
	)
	ws.Route(
		ws.GET("/instance/{instance}/state").
			Param(restful.PathParameter("instance", "instance name")).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", InstanceStateGet{}).
			Returns(404, "Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetInstanceState),
	)
	ws.Route(
		ws.PUT("/instance/{instance}/state").
			Param(restful.PathParameter("instance", "instance name")).
			Reads(InstanceStatePut{}).
			Filter(s.filterAuth("user")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(404, "Not Found", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.PutInstanceState),
	)
	ws.Route(
		ws.GET("/instance-type/{type}/queue-time").
			Param(restful.PathParameter("type", "instance type name")).
			Param(restful.QueryParameter("time", "observation time")).
			Filter(s.filterAuth("banned")).
			Returns(200, "OK", QueueTimeGet{}).
			Returns(404, "Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetEstimatedQueueTime),
	)
	ws.Route(
		ws.GET("/instance-type").
			Filter(s.filterAuth("banned")).
			Returns(200, "OK", InstanceTypeGet{}).
			Returns(500, "Internal Server Error", nil).
			To(s.GetInstanceTypes),
	)
	ws.Route(
		ws.GET("/instance-type/{type}").
			Param(restful.PathParameter("type", "instance type name")).
			Filter(s.filterAuth("banned")).
			Returns(200, "OK", []InstanceTypeGet{}).
			Returns(404, "Not Found", nil).
			Returns(500, "Internal Server Error", nil).
			To(s.GetInstanceType),
	)
	ws.Route(
		ws.PUT("/instance-type").
			Reads(InstanceTypePut{}).
			Filter(s.filterAuth("admin")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.PutInstanceType),
	)
	ws.Route(
		ws.DELETE("/instance-type/{type}").
			Param(restful.PathParameter("type", "instance type name")).
			Filter(s.filterAuth("admin")).
			Returns(200, "OK", GeneralResponse{}).
			Returns(500, "Internal Server Error", nil).
			To(s.DeleteInstanceType),
	)

	rc := restful.NewContainer()
	rc.ServeMux = mux
	rc.Add(ws)

	config := restfulspec.Config{
		WebServices:                   rc.RegisteredWebServices(), // you control what services are visible
		APIPath:                       "/apidocs.json",
		PostBuildSwaggerObjectHandler: nil}
	rc.Add(restfulspec.NewOpenAPIService(config))

	mux.HandleFunc("/ws/v1/spice", s.HandleSpiceWs)
	mux.HandleFunc("/ws/v1/exec", s.HandleExecWs)
	mux.HandleFunc("/ws/v1/console", s.HandleConsoleWs)
	mux.HandleFunc("/webdav/", s.HandleWebDAV)

	log.Println("listening on", s.conf.Listen)
	return http.ListenAndServe(s.conf.Listen, mux)
}
