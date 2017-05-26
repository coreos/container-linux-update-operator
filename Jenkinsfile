#!groovy

properties([
    buildDiscarder(logRotator(daysToKeepStr: '20', numToKeepStr: '30')),

    [$class: 'GithubProjectProperty',
     projectUrlStr: 'https://github.com/coreos/container-linux-update-operator'],

    pipelineTriggers([
      // Pull requests, with whitelisting/auth
      [$class: 'GhprbTrigger',
       cron: '*/1 * * * *',
       permitAll: false,
       orgWhitelist(['coreos', 'coreos-inc']),
       displayBuildErrorsOnDownstreamBuilds: true,
       gitHubAuthId: 'dd528eca-0dbc-4c17-a4c6-8e8a2ba7f43d'],
    ])
])

node('docker') {
  stage('SCM') {
    checkout scm
  }
  stage('Test') {
    sh "docker run --rm -u \"\$(id -u):\$(id -g)\" -v /etc/passwd:/etc/passwd:ro -v /etc/group:/etc/group:ro -v \"\$PWD\":/go/src/github.com/coreos/container-linux-update-operator -w /go/src/github.com/coreos/container-linux-update-operator golang:1.8.1 make all test"
  }
}
