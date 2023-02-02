package server

import (
	"log"
	"time"

	"github.com/lcpu-dev/vmsched/models"
)

func (s *Server) StartCron() {
	ticker := time.NewTicker(s.conf.CronInterval)
	for {
		<-ticker.C
		tasks := []*models.Task{}
		err := s.orm.Where("status = ?", "active").And("end_time < ?", time.Now().Add(-30*time.Second)).Find(&tasks)
		if err != nil {
			log.Println("ERROR:", err)
			continue
		}
		for _, task := range tasks {
			if task.EndTime.After(time.Now().Add(-30 * time.Second)) {
				continue
			}
			err = s.killTask(task)
			if err != nil {
				log.Println("ERROR:", err)
			}
		}
	}
}
