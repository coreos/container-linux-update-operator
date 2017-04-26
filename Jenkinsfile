node('docker') {
  stage('SCM') {
    checkout scm
  }
  stage('Test') {
    sh "docker run --rm -u \"\$(id -u):\$(id -g)\" -v /etc/passwd:/etc/passwd:ro -v /etc/group:/etc/group:ro -v \"\$PWD\":/go/src/github.com/coreos/container-linux-update-operator -w /go/src/github.com/coreos/container-linux-update-operator golang:1.8.1 make all test"
  }
}
