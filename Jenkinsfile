pipeline {
  agent none

  options {
    timeout(time:10, unit:'MINUTES')
    buildDiscarder(logRotator(numToKeepStr:'20'))
  }

  stages {
    stage('Test') {
      agent {
        label 'docker'
      }
      steps {
        checkout scm
        sh "docker run --rm -u \"\$(id -u):\$(id -g)\" -v /etc/passwd:/etc/passwd:ro -v /etc/group:/etc/group:ro -v \"\$PWD\":/go/src/github.com/coreos/container-linux-update-operator -w /go/src/github.com/coreos/container-linux-update-operator golang:1.8.4 make all test"
      }
    }
  }
}
