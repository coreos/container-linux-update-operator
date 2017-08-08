#!/bin/bash

set -x
set -o pipefail

function die {
	echo "$@"
	exit 1
}

# depends on kubectl and jq and sed..
type sed &>/dev/null || die "missing sed"
type jq &>/dev/null || die "missing jq"
type kubectl &>/dev/null || die "missing kubectl"

test -r "${KUBECONFIG}" || die "unable to read kubeconfig from '${KUBECONFIG}'"

test -n "${OPERATOR_IMAGE}" || die "OPERATOR_IMAGE is unset"

function first_non_master {
	kubectl get -l 'master!=true' -o json nodes | jq -r .items[0].metadata.name
}

function node_boot_id {
	kubectl  get -o json nodes/$1 | jq -r .status.nodeInfo.bootID
}

TEST_NODE=$(first_non_master)

test -n "${TEST_NODE}" || die "no non-master node available for test"

BOOT_ID=$(node_boot_id ${TEST_NODE})

test -n "${BOOT_ID}" || die "unable to get boot id of node ${TEST_NODE}"

echo "Labeling node ${TEST_NODE} for reboot"

# set a label on the node we want to reboot
kubectl label nodes "${TEST_NODE}" "alpha.coreos.com/update1.test-reboot=true"

# template manifests
sed --expression "s,@@OPERATOR_IMAGE@@,${OPERATOR_IMAGE},g" ./hack/update-operator.tmpl > ./hack/update-operator.yaml

### XXX: paranoia
echo $PWD
ls -l hack

# create our operator
kubectl --namespace=kube-system create -f ./hack/update-operator.yaml

SECONDS=0
until kubectl get thirdpartyresources reboot-group.coreos.com &>/dev/null || (( SECONDS > 30 )); do
	sleep 2
done

kubectl get thirdpartyresources reboot-group.coreos.com &>/dev/null || die "timed out waiting for tpr creation"
AGENT_POD_COUNT=$(kubectl --namespace=kube-system get -l app=update-agent -o json | jq -r ".items | map(select(.spec.nodeName==\"${TEST_NODE}\")) | length")

if [ "${AGENT_POD_COUNT}" -lt 1 ]; then
	die "timed out waitinf for update-agent to run on ${TEST_NODE}"
fi

# fire off our reboot job

echo "Rebooting ${TEST_NODE}"

kubectl --namespace=kube-system create -f ./hack/reboot-job.yaml

SECONDS=0
until [ "$(node_boot_id ${TEST_NODE})" != "${BOOT_ID}" ] || SECOONDS > 300 )); do
	sleep 15
done

if [ "$(node_boot_id ${TEST_NODE})" == "${BOOT_ID}" ]; then
	die "boot id has failed to change in 300 seconds"
fi

echo "i think it worked"

