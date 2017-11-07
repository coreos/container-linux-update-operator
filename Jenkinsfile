#!groovy

properties([
    buildDiscarder(logRotator(daysToKeepStr: '20', numToKeepStr: '30')),

    [$class: 'GithubProjectProperty',
     projectUrlStr: 'https://github.com/coreos/container-linux-update-operator'],

    pipelineTriggers([
      // Pull requests, with whitelisting/auth
      [$class: 'GhprbTrigger',
       cron: 'H/5 * * * *',
       permitAll: false,
       orgslist: 'coreos',
       displayBuildErrorsOnDownstreamBuilds: true,],
    ])
])

node('docker') {
  stage('SCM') {
    checkout scm
  }
  stage('Test') {
    sh "docker run --rm -u \"\$(id -u):\$(id -g)\" -v /etc/passwd:/etc/passwd:ro -v /etc/group:/etc/group:ro -v \"\$PWD\":/go/src/github.com/coreos/container-linux-update-operator -w /go/src/github.com/coreos/container-linux-update-operator golang:1.8.4 make all test"
  }
}
