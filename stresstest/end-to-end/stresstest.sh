#!/usr/bin/env bash

######
# Getopt setup
######
! getopt --test > /dev/null
if [[ ${PIPESTATUS[0]} -ne 4 ]]; then
    echo "I'm sorry, 'getopt --test' failed in this environment."
    exit 1
fi

LONGOPTS=help,verbose,mode:,use-vni,num-jobs:,iterations:,ramp-min:,ramp-max:,ramp-sustain:,ramp-step-wait:,ramp-step-size:,namespace:,kubectl:
OPTIONS=
! PARSED=$(getopt --options=$OPTIONS --longoptions=$LONGOPTS \
	--name "$0" -- "$@")
if [[ ${PIPESTATUS[0]} -ne 0 ]]; then    # e.g. return value is 1
  #  then getopt has complained about wrong arguments to stdout
  exit 2
fi

######
# Global Variables
######
OUTDIR="$(pwd)/data/"
KUBECTL="/opt/k3s/k3s-arm64 kubectl"
NAMESPACE="stresstest"
NUMJOBS="1"
NUMITER="1"
MODE="spike-test"
RAMP_MIN=1
RAMP_MAX=5
RAMP_STEP_SIZE=1
RAMP_STEP_WAIT_S=1
RAMP_SUSTAIN=10
USE_VNI="false"

TMPSTORAGE=/dev/shm/stresstest

LAUNCHED_JOBS=()
BATCH_SIZES=()
START_TIMES=()

######
# Functions
######
function show_help() {
  cat << EOF
Run stresstest

Options:
 --outdir <dir>                  Path to directory for output files (default: $OUTDIR)
 --mode <mode>                   Either 'spike-test' or 'ramp-test' (default: 'spike-test')
 --use-vni                       Use VNI stack by setting "vni: true" annotation (default: $USE_VNI)
 --num-jobs <n>                  Number of jobs to launch (default: $NUMJOBS)
 --iterations <n>                Number of outer iterations (default: $NUMITER)
 --ramp-min <n>                  Starting number of jobs during ramp-up time, for ramp-test (default: $RAMP_MIN)
 --ramp-max <n>                  Max number of jobs during ramp-up, for ramp-test (default: $RAMP_MAX)
 --ramp-sustain <steps>          Number of steps during ramp-sustain, for ramp-test (default: $RAMP_SUSTAIN)
 --ramp-step-wait <sec>          Waiting time between ramp steps (default: $RAMP_STEP_WAIT_S)
 --ramp-step-size <n>            Number of steps per ramp-up/-down step (default: $RAMP_STEP)
 --namespace <namespace>         Namespace for jobs (default: $NAMESPACE)
 --kubectl <path/to/kubectl>     Path to kubectl binary (default: $KUBECTL)
 --help
 --verbose

Note:
  For spike-test, --num-jobs controls the total number of jobs to be launched at once.
  For ramp-up-test, --num-jobs controls the number of jobs per --iterations.
EOF
}

function printVariables() {
    echo "OUTDIR          : $OUTDIR"
    echo "BASEPATH        : $BASEPATH"
    echo "KUBECTL         : $KUBECTL"
    echo "NAMESPACE       : $NAMESPACE"
    echo "NUMJOBS         : $NUMJOBS"
    echo "NUMITER         : $NUMITER"
    echo "MODE            : $MODE"
    echo "RAMP_MIN        : $RAMP_MIN"
    echo "RAMP_MAX        : $RAMP_MAX"
    echo "RAMP_STEP_WAIT_S: $RAMP_STEP_WAIT_S"
    echo "RAMP_SUSTAIN    : $RAMP_SUSTAIN"
    echo "RAMP_STEP_SIZE  : $RAMP_STEP_SIZE"
}

function generateJob() {
  local numJobs=$1

  local job=""
  for i in  $(seq 1 "$numJobs"); do
    if [[ $USE_VNI == "true" ]]; then
    read -r -d '' job <<EOF
$job
---
apiVersion: batch/v1
kind: Job
metadata:
  generateName: $NAMESPACE-job-
  namespace: $NAMESPACE
  annotations:
    vni: "true"
spec:
  template:
    spec:
      containers:
      - name: stresstest
        image: aam1.caps.cit.tum.de:9443/containerssh-image-friese:latest
        command: ["echo"]
      restartPolicy: Never
      nodeSelector:
        kubernetes.io/hostname: i10se19
  backoffLimit: 4

EOF
  else
        read -r -d '' job <<EOF
$job
---
apiVersion: batch/v1
kind: Job
metadata:
  generateName: $NAMESPACE-job-
  namespace: $NAMESPACE
spec:
  template:
    spec:
      containers:
      - name: stresstest
        image: aam1.caps.cit.tum.de:9443/containerssh-image-friese:latest
        command: ["echo"]
      restartPolicy: Never
      nodeSelector:
        kubernetes.io/hostname: i10se19
  backoffLimit: 4

EOF
  fi
  done
  echo "$job"
}

function writeMeta() {
  local meta_info
  read -r -d '' meta_info << EOF
Measurement Start: $(date +%c) ($(date -u +%s))
Host: $(hostname) $(uname -a)
kubectl: \`$KUBECTL\` ($($KUBECTL version 2>/dev/null|tr '\n' ' '))
Variables:
$(printVariables|awk '{print " " $0}')
EOF
  echo "$meta_info" > "$BASEPATH/meta.md"
}

function spikeTest() {
  local iteration="$1"
  local cleaningStartTime
  local cleaningEndTime

  echo "Generating jobs"
  generateJob "$NUMJOBS" > "$TMPSTORAGE/$NUMJOBS.yml"
  launchJobs "$NUMJOBS" 0

  echo "Waiting for jobs to complete."
  $KUBECTL -n "$NAMESPACE" wait --timeout=-1s --for=condition=Complete "${LAUNCHED_JOBS[@]}" 1>/dev/null
  echo "Done. Fetching job data and writing to disk."
  $KUBECTL -n "$NAMESPACE" get jobs -o json|gzip > "$BASEPATH/$iteration-job_output.json.gz"

  echo "Cleaning up jobs"
  cleaningStartTime=$(date -u +%s)
  $KUBECTL -n "$NAMESPACE" delete job --all 1>/dev/null
  cleaningEndTime=$(date -u +%s)

  local job_data
  read -r -d '' job_data << EOF
LAUNCHED_JOBS: ${LAUNCHED_JOBS[@]}
BATCH_SIZES: ${BATCH_SIZES[@]}
START_TIMES: ${START_TIMES[@]}
CLEAN_START: $cleaningStartTime
CLEAN_END: $cleaningEndTime
EOF
  echo "$job_data" > "$BASEPATH/$iteration-job_data"
}

function launchJobs() {
  local numJobs=$1
  local batch=$2
  local jobsCreated

  START_TIMES+=($(($(date +%s%N)/1000000)))
  mapfile -t jobsCreated <<< $($KUBECTL create -f "$TMPSTORAGE/$numJobs.yml" | awk '{print $1}')
  LAUNCHED_JOBS+=("${jobsCreated[@]}")
  BATCH_SIZES+=("$numJobs")
}

function generateJobFiles() {
  local numJobs

    for step in $(seq "$RAMP_STEP_SIZE" "$RAMP_STEP_SIZE" "$RAMP_MAX"); do
      numJobs=$step
      jobs=$(generateJob "$numJobs")
      echo "$jobs" > "$TMPSTORAGE/$numJobs.yml"
    done

    jobs=$(generateJob "$RAMP_MAX")
    echo "$jobs" > "$TMPSTORAGE/$RAMP_MAX.yml"

    for step in $(seq "$RAMP_STEP_SIZE" "$RAMP_STEP_SIZE" "$RAMP_MAX"); do
      numJobs=$(( RAMP_MAX - step ))
      if [[ $numJobs == "0" ]]; then
        continue
      fi
      if [[ ! -f "$TMPSTORAGE/$numJobs.yml" ]]; then
        jobs=$(generateJob "$numJobs")
        echo "$jobs" > "$TMPSTORAGE/$numJobs.yml"
      fi
    done

}

function rampTest() {
  local batch
  local numJobs
  local iteration
  iteration="$1"

  generateJobFiles

  batch=1
  for step in $(seq "$RAMP_STEP_SIZE" "$RAMP_STEP_SIZE" "$RAMP_MAX"); do
    numJobs=$step
    echo "[$(date -u +%s)|$(printf "%3s" $batch)] Step ramp-up @ $numJobs jobs"
    launchJobs "$numJobs" "$batch"
    batch=$(( batch + 1 ))
    sleep "$RAMP_STEP_WAIT_S"
  done

  for step in $(seq 1 "$RAMP_SUSTAIN"); do
    numJobs=$RAMP_MAX
    echo "[$(date -u +%s)|$(printf "%3s" $batch)] Step sustain @ $numJobs jobs"
    launchJobs "$numJobs" "$batch"
    batch=$(( batch + 1 ))
    sleep "$RAMP_STEP_WAIT_S"
  done

  for step in $(seq "$RAMP_STEP_SIZE" "$RAMP_STEP_SIZE" "$RAMP_MAX"); do
    numJobs=$(( RAMP_MAX - step ))
    if [[ $numJobs == "0" ]]; then
      continue
    fi
    echo "[$(date -u +%s)|$(printf "%3s" $batch)] Step ramp-down @ $numJobs jobs"
    launchJobs "$numJobs" "$batch"
    batch=$(( batch + 1 ))
    sleep "$RAMP_STEP_WAIT_S"
  done

  echo "Waiting for jobs to complete."
  $KUBECTL -n "$NAMESPACE" wait --timeout=-1s --for=condition=Complete "${LAUNCHED_JOBS[@]}" 1>/dev/null
  echo "Done. Fetching job data and writing to disk."
  $KUBECTL -n "$NAMESPACE" get jobs -o json|gzip > "$BASEPATH/$iteration-job_output.json.gz"


  echo "$job_data" > "$BASEPATH/$iteration-job_data"

  echo "Cleaning up jobs"
  cleaningStartTime=$(date -u +%s)
  $KUBECTL -n "$NAMESPACE" delete job --all 1>/dev/null
  cleaningEndTime=$(date -u +%s)

  local job_data
  read -r -d '' job_data << EOF
LAUNCHED_JOBS: ${LAUNCHED_JOBS[@]}
BATCH_SIZES: ${BATCH_SIZES[@]}
START_TIMES: ${START_TIMES[@]}
CLEAN_START: $cleaningStartTime
CLEAN_END: $cleaningEndTime
EOF
}

######
# Start Script
######
eval set -- "$PARSED"
while true; do
  case "$1" in
    -h|--help)
      show_help
      exit 1 ;;
    -v|--verbose)
      VERBOSE=1
      shift ;;
    --mode)
      if [[ $2 == "spike-test" || $2 == "ramp-test" ]]; then
        MODE="$2"
      else
        echo "Invalid --mode: $2 (can only be: 'spike-test' and 'ramp-test')"
        exit 2
      fi
      shift 2;;
    --use-vni)
      USE_VNI="true"
      shift ;;
    --num-jobs)
      NUMJOBS="$2"
      shift 2 ;;
    --iterations)
      NUMITER="$2"
      shift 2 ;;
    --ramp-min)
      RAMP_MIN="$2"
      shift 2 ;;
    --ramp-max)
      RAMP_MAX="$2"
      shift 2 ;;
    --ramp-sustain)
      RAMP_SUSTAIN="$2"
      shift 2 ;;
    --ramp-step-wait)
      RAMP_STEP_WAIT_S="$2"
      shift 2 ;;
    --ramp-step-size)
      RAMP_STEP_SIZE="$2"
      shift 2 ;;
    --namespace)
      NAMESPACE="$2"
      shift 2 ;;
    --kubectl)
      KUBECTL="$2"
      shift 2 ;;
    --)
      shift
      break
      ;;
    *)
      echo "Missing parameters"
      show_help
      exit 3
      ;;
    esac
done

BASEDIR="measurements_$(date +%y-%m-%dT%H%M)"
OUTDIR="$(pwd)/data"
mkdir --parents "$OUTDIR"

# if $OUTDIR/$BASEDIR exists, try $OUTDIR/$BASEDIR-$i until not exists
_basedir="$BASEDIR"
i=1
while true; do
  if [[ -d "$OUTDIR/$_basedir" ]]; then
        _basedir="$BASEDIR-$i"
    i=$((i+1))
  else break
  fi
done
BASEDIR="$_basedir"
BASEPATH="$OUTDIR/$BASEDIR"
mkdir --parents "$BASEPATH"

if [[ $VERBOSE -gt 0 ]]; then
  echo "Variables:"
  printVariables
  echo ""
fi

echo "Starting Stresstest"
echo " writing to $BASEPATH"
mkdir --parents $TMPSTORAGE

writeMeta

for n in $(seq 1 "$NUMITER"); do
  LAUNCHED_JOBS=()
  BATCH_SIZES=()
  START_TIMES=()
  
  echo "Iteration $n, mode $MODE"
  if [[ $MODE == "spike-test" ]]; then
    spikeTest "$n"
echo
  elif [[ $MODE == "ramp-test" ]]; then
    rampTest "$n"
  fi
done
echo "Stresstest done."
