package sshbox

import "github.com/sirupsen/logrus"

var logger *logrus.Logger = logrus.StandardLogger()

func SetLogger(newLogger *logrus.Logger) {
	logger = newLogger
}
