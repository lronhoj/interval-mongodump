package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

/*
	plan:
	- stop cron backup, planet backends
		$ docker stop planet_admin_1 planet_backend_1 planet_backend_2 planet_cron-backup_1
	- force backup
		$ BACKUP_DIR=/home/jenkins/backup docker-compose -f docker-compose.backup.yml run backup-images
		$ BACKUP_DIR=/home/jenkins/backup docker-compose -f docker-compose.backup.yml run backup-certs
		$ docker rm -f planet_backup-certs_run_1 planet_backup-images_run_1 planet_cron-backup_1
	- remove containers
		$ DOMAIN_NAME=planet.arosii.com TAG=1.16.3 BACKUP_DIR=/home/jenkins/backup docker-compose -f docker-compose.yml -f docker-compose.ext.cloud.yml stop webserver imageservice
	- deploy
*/

func main() {
	host := os.Getenv("HOST")
	rawDuration := os.Getenv("TICK")
	retention64, err := strconv.ParseInt(os.Getenv("RETENTION"), 10, 32)
	if err != nil {
		log.Fatal(fmt.Sprintf("$RETENTION must be an integer: %v", err))
	}
	retention := int(retention64)
	if retention < 0 {
		log.Fatal("$RETENTION must be a positive integer")
	}
	if host == "" {
		log.Fatal("$HOST must be provided")
	}

	err = run(host, retention)
	if rawDuration == "" {
		if err != nil {
			os.Exit(1)
		}
		return
	}

	if rawDuration == "once" {
		run(host, retention)
		os.Exit(0)
	}

	duration, err := time.ParseDuration(rawDuration)
	if err != nil {
		log.Fatal("$TICK is not a valid format: See https://golang.org/pkg/time/#ParseDuration")
	}

	ticker := time.NewTicker(duration)

	go func() {
		for range ticker.C {
			run(host, retention)
		}
	}()

	select {}

}

func run(host string, retention int) error {
	err := dump(host)
	if err != nil {
		log.Printf("mongodump failed: %v\n", err)
		return err
	}

	err = removeWeekOldBackup(retention)
	if err != nil {
		log.Printf("backup remove failed: %v\n", err)
		return err
	}
	return nil
}

func dump(host string) error {
	now := time.Now()
	path := fmt.Sprintf("/backup/backup-%s", df(now))

	cmdName := "mongodump"
	cmdArgs := []string{"--host", host, "--out", path}

	cmd := exec.Command(cmdName, cmdArgs...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdoutPipe)
	go func() {
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	errScanner := bufio.NewScanner(stderrPipe)
	go func() {
		for errScanner.Scan() {
			log.Println(errScanner.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return err
	}
	log.Println("Backup completed successfully")

	return nil
}

func removeWeekOldBackup(retention int) error {
	old := time.Now().AddDate(0, 0, -retention)
	path := fmt.Sprintf("/backup/backup-%s", df(old))

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}

	if fileInfo.IsDir() {
		log.Printf("Removing week old backup at %s\n", path)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	return nil
}

func df(time time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d", time.Year(), time.Month(), time.Day())
}
