package server

import (
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/emicklei/go-restful/v3"
	"github.com/lcpu-dev/vmsched/models"
	"github.com/lcpu-dev/vmsched/renderer"
	"github.com/lxc/lxd/shared/api"
	"xorm.io/xorm"
)

func (s *Server) PutUser(req *restful.Request, resp *restful.Response) {
	userPut := &UserPut{}
	err := req.ReadEntity(userPut)
	if err != nil || userPut.Name == "" {
		resp.WriteHeaderAndEntity(400, &GeneralResponse{Success: false, Message: err.Error()})
		return
	}
	user := &models.User{
		Name: userPut.Name,
	}
	exists, err := s.orm.Get(user)
	if err != nil {
		resp.WriteHeaderAndEntity(500, &GeneralResponse{Success: false, Message: err.Error()})
		return
	}
	user.Role = userPut.Role
	user.Balance = userPut.Balance
	if !exists {
		_, err = s.orm.Insert(user)
		if err != nil {
			resp.WriteHeaderAndEntity(500, &GeneralResponse{Success: false, Message: err.Error()})
			return
		}
	} else {
		_, err = s.orm.Update(user, &models.User{Name: userPut.Name})
		if err != nil {
			resp.WriteHeaderAndEntity(500, &GeneralResponse{Success: false, Message: err.Error()})
			return
		}
	}
	resp.WriteEntity(&GeneralResponse{Success: true})
}

func (s *Server) GetUser(req *restful.Request, resp *restful.Response) {
	user := ""
	var ok bool
	if user, ok = req.PathParameters()["user"]; !ok {
		u := req.Attribute("user")
		user = u.(string)
	}
	u := &models.User{Name: user}
	ok, err := s.orm.Get(u)
	if err != nil {
		resp.WriteError(500, err)
	}
	if !ok {
		resp.WriteErrorString(404, "Not Found")
	}
	resp.WriteEntity(&UserPut{
		Name:    u.Name,
		Role:    u.Role,
		Balance: u.Balance,
	})
}

func (s *Server) GetUserTasks(req *restful.Request, resp *restful.Response) {
	u := req.PathParameter("user")
	tasks := []*models.Task{}
	err := s.orm.Find(&tasks, &models.Task{User: u})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	rslt := []*TaskGet{}
	for _, t := range tasks {
		rslt = append(rslt, &TaskGet{
			Name:         t.Name,
			Instance:     t.Instance,
			InstanceType: t.InstanceType,
			Status:       t.Status,
			Creation:     t.Creation,
			QueueTime:    t.QueueTime,
			EndTime:      t.EndTime,
		})
	}
	resp.WriteEntity(rslt)
}

func (s *Server) GetTask(req *restful.Request, resp *restful.Response) {
	task := req.PathParameter("task")
	tsk := &models.Task{Name: task}
	ok, err := s.orm.Get(tsk)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "task not found"})
		return
	}
	resp.WriteEntity(&TaskGet{
		Name:         tsk.Name,
		Instance:     tsk.Instance,
		InstanceType: tsk.InstanceType,
		Status:       tsk.Status,
		Creation:     tsk.Creation,
		QueueTime:    tsk.QueueTime,
		EndTime:      tsk.EndTime,
	})
}

func (s *Server) PostUserTask(req *restful.Request, resp *restful.Response) {
	u := req.PathParameter("user")
	task := &TaskPost{}
	err := req.ReadEntity(task)
	if err != nil || task.Name == "" || task.InstanceType == "" {
		resp.WriteErrorString(400, "Bad Request")
		return
	}
	ok, err := s.orm.Exist(&models.Task{Name: task.Name})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if ok {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "task already exists"})
		return
	}
	typ := &models.InstanceType{Name: task.InstanceType}
	if ok, err = s.orm.Get(typ); err == nil {
		if !ok {
			resp.WriteEntity(&GeneralResponse{Success: false, Message: "instance type not found"})
			return
		}
	} else {
		resp.WriteError(500, err)
		return
	}
	insConf, err := renderer.YAMLToInstancePost(typ.Configure)
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "invalid instance configure"})
		return
	}
	r, err := renderer.NewRenderer(s.lxd, map[string]interface{}{})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	target := &models.InstanceTarget{Type: task.InstanceType}
	if ok, err = s.orm.Desc("status").Get(target); err == nil {
		if !ok {
			resp.WriteEntity(&GeneralResponse{Success: false, Message: "instance type not found"})
			return
		}
	} else {
		resp.WriteError(500, err)
		return
	}
	// TODO: better name generating
	insName := "task-" + task.Name + "-" + strconv.Itoa(rand.Intn(99999999))
	tsk := &models.Task{
		Name:         task.Name,
		InstanceType: task.InstanceType,
		Creation:     time.Now(),
		EndTime:      time.Time{},
		Status:       "creating",
		TargetID:     target.Id,
		Instance:     insName,
		User:         u,
	}
	_, err = s.orm.Insert(tsk)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	insConf.Name = insName
	err = r.RenderCreate(insConf, target.Target)
	if err != nil {
		s.orm.Delete(tsk)
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "instance creation error: " + err.Error()})
		return
	}
	tsk.Status = "inactive"
	_, err = s.orm.Update(tsk, &models.Task{Name: tsk.Name})
	if err != nil {
		resp.WriteError(500, err)
	} else {
		resp.WriteEntity(&GeneralResponse{Success: true})
	}
}

func (s *Server) PostTaskState(req *restful.Request, resp *restful.Response) {
	task := req.PathParameter("task")
	stt := &TaskStatePost{}
	err := req.ReadEntity(stt)
	if err != nil {
		resp.WriteError(400, err)
		return
	}
	if stt.Status != "active" {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "action not supported"})
		return
	}
	lifetime, err := time.ParseDuration(stt.LifeTime)
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "invalid lifetime"})
		return
	}
	if lifetime < time.Minute {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "life time too short"})
		return
	}
	t := &models.Task{Name: task}
	ok, err := s.orm.Get(t)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "task not found"})
		return
	}
	if t.Status != "inactive" {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "cannot operate non inactive task"})
		return
	}
	u := &models.User{Name: t.User}
	ok, err = s.orm.Get(u)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "user not found"})
		return
	}
	it := &models.InstanceType{Name: t.InstanceType}
	ok, err = s.orm.Get(it)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "instance type not found"})
		return
	}
	for k, v := range it.Price {
		price := v * int(lifetime/time.Minute)
		if val, ok := u.Balance[k]; (ok) && (val >= price) {
			u.Balance[k] -= price
		} else {
			resp.WriteEntity(&GeneralResponse{Success: false, Message: "balance is low"})
			return
		}
	}
	affectedRows, err := s.orm.Update(u, &models.User{Name: u.Name})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if affectedRows <= 0 {
		resp.WriteHeaderAndEntity(500, &GeneralResponse{Success: false, Message: "probable concurrent write"})
		return
	}
	t.Status = "queued"
	t.QueueTime = time.Now()
	_, err = s.orm.Update(t, &models.Task{Name: t.Name})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	ok, err = s.activateTask(t, lifetime, nil)
	if err != nil {
		log.Println("ERROR:", err)
		resp.WriteError(500, err)
		return
	}
	if ok {
		resp.WriteEntity(&GeneralResponse{Success: true, Message: "active"})
	} else {
		// put into queue
		q := &models.Queue{
			User:         t.User,
			Task:         t.Name,
			LifeTime:     lifetime,
			InstanceType: t.InstanceType,
		}
		_, err = s.orm.Insert(q)
		if err != nil {
			log.Println("ERROR:", err)
			resp.WriteError(500, err)
			return
		}
		resp.WriteEntity(&GeneralResponse{Success: true, Message: "queued"})
	}
}

func (s *Server) activateTask(task *models.Task, lifetime time.Duration, tgt *models.InstanceTarget) (bool, error) {
	// FIXME: Dequeue and Requeue logic should be processed outside
	// _, err = s.orm.Delete(&models.Queue{Task: task.Name})
	// if err != nil {
	// 	return false, err
	// }
	it := &models.InstanceType{Name: task.InstanceType}
	ok, err := s.orm.Get(it)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("instance type not found")
	}
	conf, err := renderer.YAMLToInstancePost(it.Configure)
	if err != nil {
		return false, err
	}
	r, err := renderer.NewRenderer(s.lxd, map[string]interface{}{})
	if err != nil {
		return false, err
	}
	// Get and set target
	var target *models.InstanceTarget
	if tgt == nil {
		target = &models.InstanceTarget{Type: task.InstanceType, Status: "idle"}
		ok, err = s.orm.Get(target)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		target.Status = "busy"
		target.Instance = task.Instance
		target.Task = task.Name
		affectedRows, err := s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		if err != nil {
			return false, err
		}
		if affectedRows <= 0 {
			return false, fmt.Errorf("probable concurrent write")
		}
	} else {
		target = tgt
	}
	err = r.RenderStart(task.Instance, conf.InstancePut, target.Target)
	if err != nil {
		target.Status = "idle"
		s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		return false, err
	}
	timeout := lifetime
	go func() {
		time.Sleep(timeout)
		err := s.killTask(task)
		if err != nil {
			log.Println("ERROR:", err)
		}
	}()
	task.Status = "active"
	task.EndTime = time.Now().Add(lifetime)
	task.TargetID = target.Id
	_, err = s.orm.Update(task, &models.Task{Name: task.Name})
	if err != nil {
		return true, err
	}
	return true, nil
}

func (s *Server) killTask(task *models.Task) error {
	log.Println("killing task", task)
	task.Status = "terminating"
	affectedRows, err := s.orm.Update(task, &models.Task{Name: task.Name})
	if err != nil {
		return err
	}
	if affectedRows <= 0 {
		return nil
	}
	op, err := s.lxd.UpdateInstanceState(task.Instance, api.InstanceStatePut{
		Action:   "stop",
		Force:    false,
		Stateful: true,
	}, "")
	if err != nil {
		if !strings.Contains(err.Error(), "already stopped") {
			return err
		}
	}
	if err == nil {
		err = op.Wait()
		if err != nil && !strings.Contains(err.Error(), "already stopped") {
			if strings.Contains(err.Error(), "migration.stateful") || strings.Contains(err.Error(), "install CRIU") {
				op, err = s.lxd.UpdateInstanceState(task.Instance, api.InstanceStatePut{
					Action:   "stop",
					Force:    true,
					Stateful: false,
				}, "")
				if err != nil && !strings.Contains(err.Error(), "already stopped") {
					return err
				}
				if err == nil {
					err = op.Wait()
					if err != nil && !strings.Contains(err.Error(), "already stopped") {
						return err
					}
				}
			} else {
				return err
			}
		}
	}
	task.Status = "inactive"
	_, err = s.orm.Update(task, &models.Task{Name: task.Name})
	if err != nil {
		return err
	}
	log.Println("killed task", task)
	// FIXME: should execute the after things
	target := &models.InstanceTarget{Id: task.TargetID}
	ok, err := s.orm.Get(target)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	queueItem := &models.Queue{}
	ok, err = s.orm.Where("instance_type = ?", task.InstanceType).Asc("creation").Get(queueItem)
	if err != nil {
		return err
	}
	if !ok {
		// free the targets
		target.Status = "idle"
		_, err = s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		return err
	}
	_, err = s.orm.Delete(queueItem)
	if err != nil {
		// free the targets
		target.Status = "idle"
		_, err = s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		return err
	}
	nt := &models.Task{Name: queueItem.Task}
	ok, err = s.orm.Get(task)
	if err != nil {
		// free the targets
		target.Status = "idle"
		_, err = s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		return err
	}
	if !ok {
		// free the targets
		target.Status = "idle"
		_, err = s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		return err
	}
	log.Println("starting task", nt, "on", target)
	ok, err = s.activateTask(nt, queueItem.LifeTime, target)
	if err != nil || !ok {
		target.Status = "idle"
		_, err = s.orm.Update(target, &models.InstanceTarget{Id: target.Id})
		if err != nil {
			return err
		}
		// Requeue
		_, err = s.orm.Insert(queueItem)
		if err != nil {
			return err
		}
	}
	log.Println("started task", nt, "on", target)
	return nil
}

func (s *Server) estimateQueueTime(instanceType string, creationBefore time.Time) (time.Duration, error) {
	queues := []*models.Queue{}
	err := s.orm.Where("instance_type = ?", instanceType).Where("creation < ?", creationBefore).Find(queues)
	if err != nil {
		return 0, err
	}
	total := time.Duration(0)
	for _, qi := range queues {
		total += qi.LifeTime
	}
	if ok, _ := s.orm.Exist(&models.InstanceTarget{Type: instanceType, Status: "idle"}); !ok {
		leastTask := &models.Task{InstanceType: instanceType, Status: "active"}
		ok, err := s.orm.Desc("end_time").Get(leastTask)
		if err != nil {
			return 0, err
		}
		if !ok {
			return total, nil
		}
		total += time.Until(leastTask.EndTime)
	}
	return total, nil
}

func (s *Server) GetEstimatedQueueTime(req *restful.Request, resp *restful.Response) {
	instanceType := req.PathParameter("type")
	timeRaw := req.QueryParameter("time")
	tm := time.Now()
	if timeRaw != "" {
		var err error
		tm, err = time.Parse(time.RFC3339, timeRaw)
		if err != nil {
			resp.WriteError(400, err)
			return
		}
	}
	qt, err := s.estimateQueueTime(instanceType, tm)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	resp.WriteEntity(&QueueTimeGet{
		Duration: qt.String(),
	})
}

func (s *Server) DeleteTask(req *restful.Request, resp *restful.Response) {
	task := req.PathParameter("task")
	t := &models.Task{Name: task}
	ok, err := s.orm.Get(t)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "task not found"})
		return
	}
	if t.Status != "inactive" {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "task is not inactive"})
		return
	}
	t.Status = "deleting"
	_, err = s.orm.Update(t, &models.Task{Name: task})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	op, err := s.lxd.DeleteInstance(t.Instance)
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "failed to delete instance: " + err.Error()})
		return
	}
	err = op.Wait()
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: "failed to delete instance: " + err.Error()})
		return
	}
	_, err = s.orm.Delete(t)
	if err != nil {
		resp.WriteError(500, err)
	} else {
		resp.WriteEntity(&GeneralResponse{Success: true})
	}
}

func (s *Server) GetInstanceState(req *restful.Request, resp *restful.Response) {
	instance := req.PathParameter("instance")
	state, _, err := s.lxd.GetInstanceState(instance)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if state == nil {
		resp.WriteErrorString(404, "Not Found")
		return
	}
	resp.WriteEntity(&InstanceStateGet{
		Name:        instance,
		Status:      state.Status,
		CPUUsage:    state.CPU.Usage,
		MemoryUsage: state.Memory.Usage,
	})
}

func (s *Server) PutInstanceState(req *restful.Request, resp *restful.Response) {
	instance := req.PathParameter("instance")
	entity := &InstanceStatePut{}
	err := req.ReadEntity(entity)
	if err != nil {
		resp.WriteError(400, err)
		return
	}
	if entity.Action != "start" && entity.Action != "stop" && entity.Action != "restart" {
		resp.WriteHeaderAndEntity(400, &GeneralResponse{Success: false, Message: "unknown action"})
		return
	}
	op, err := s.lxd.UpdateInstanceState(instance, api.InstanceStatePut{
		Action:   entity.Action,
		Force:    entity.Force,
		Stateful: entity.Stateful,
	}, "")
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: err.Error()})
		return
	}
	err = op.Wait()
	if err != nil {
		resp.WriteEntity(&GeneralResponse{Success: false, Message: err.Error()})
		return
	}
	resp.WriteEntity(&GeneralResponse{Success: true})
}

func (s *Server) PutUserToken(req *restful.Request, resp *restful.Response) {
	u := req.PathParameter("user")
	p := &TokenPut{}
	err := req.ReadEntity(p)
	if err != nil || p.Name == "" {
		resp.WriteError(400, err)
		return
	}
	token := &models.Token{
		Name:   p.Name,
		Secret: p.Secret,
		User:   u,
	}
	_, err = s.orm.Insert(token)
	if err != nil {
		resp.WriteError(500, err)
	} else {
		resp.WriteEntity(&GeneralResponse{Success: true})
	}
}

func (s *Server) GetUserToken(req *restful.Request, resp *restful.Response) {
	u := req.PathParameter("user")
	tokens := []*models.Token{}
	err := s.orm.Find(&tokens, &models.Token{User: u})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	rslt := TokenGet{}
	for _, val := range tokens {
		rslt = append(rslt, struct {
			Name string `json:"name"`
		}{
			Name: val.Name,
		})
	}
	resp.WriteEntity(rslt)
}

func (s *Server) DeleteUserToken(req *restful.Request, resp *restful.Response) {
	u := req.PathParameter("user")
	token := req.PathParameter("token")
	tok := &models.Token{Name: token}
	ok, err := s.orm.Get(tok)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "token not exist"})
		return
	}
	if tok.User != u {
		resp.WriteHeaderAndEntity(404, &GeneralResponse{Success: false, Message: "user and token not match"})
		return
	}
	_, err = s.orm.Delete(tok)
	if err != nil {
		resp.WriteError(500, err)
	} else {
		resp.WriteEntity(&GeneralResponse{Success: true})
	}
}

func (s *Server) GetInstanceTypes(req *restful.Request, resp *restful.Response) {
	r := []*models.InstanceType{}
	err := s.orm.Find(&r)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	rslt := []*InstanceTypeGet{}
	for _, v := range r {
		rslt = append(rslt, &InstanceTypeGet{
			Name:        v.Name,
			Description: v.Description,
			Price:       v.Price,
		})
	}
	resp.WriteEntity(rslt)
}

func (s *Server) GetInstanceType(req *restful.Request, resp *restful.Response) {
	r := &models.InstanceType{Name: req.PathParameter("type")}
	ok, err := s.orm.Get(r)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	if !ok {
		resp.WriteErrorString(404, "Not Found")
	}
	resp.WriteEntity(&InstanceTypeGet{
		Name:        r.Name,
		Description: r.Description,
		Price:       r.Price,
	})
}

func (s *Server) PutInstanceType(req *restful.Request, resp *restful.Response) {
	r := &InstanceTypePut{}
	err := req.ReadEntity(r)
	if err != nil || r.Name == "" {
		resp.WriteError(400, err)
		return
	}
	insType := &models.InstanceType{Name: r.Name}
	exists, err := s.orm.Exist(insType)
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	insType.Configure = r.Configure
	insType.Price = r.Price
	insType.Description = r.Description
	rd, err := renderer.NewRenderer(s.lxd, map[string]interface{}{})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	targets, err := renderer.ParseTargets(rd, []byte(insType.Configure))
	if err != nil {
		resp.WriteHeaderAndEntity(200, &GeneralResponse{Success: false, Message: err.Error()})
		return
	}
	_, err = s.orm.Transaction(func(session *xorm.Session) (interface{}, error) {
		if !exists {
			_, err = session.Insert(insType)
			if err != nil {
				return nil, err
			}
		} else {
			_, err = session.Update(insType, &models.InstanceType{Name: insType.Name})
			if err != nil {
				return nil, err
			}
		}
		_, err = session.Delete(&models.InstanceTarget{Type: insType.Name})
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			t := &models.InstanceTarget{
				Type:     insType.Name,
				Target:   target,
				Status:   "idle",
				Instance: "",
				Task:     "",
			}
			_, err := session.Insert(t)
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		resp.WriteError(500, err)
		return
	}
	resp.WriteEntity(&GeneralResponse{Success: true})
}

func (s *Server) DeleteInstanceType(req *restful.Request, resp *restful.Response) {
	typ := req.PathParameter("type")
	_, err := s.orm.Transaction(func(session *xorm.Session) (interface{}, error) {
		_, err := session.Delete(&models.InstanceType{Name: typ})
		if err != nil {
			return nil, err
		}
		_, err = session.Delete(&models.InstanceTarget{Type: typ})
		if err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		resp.WriteError(500, err)
	} else {
		resp.WriteEntity(&GeneralResponse{Success: true})
	}
}
