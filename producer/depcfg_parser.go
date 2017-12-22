package producer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/couchbase/eventing/common"
	"github.com/couchbase/eventing/gen/flatbuf/cfg"
	"github.com/couchbase/eventing/logging"
	"github.com/couchbase/eventing/util"
)

func (p *Producer) parseDepcfg() error {
	logging.Infof("DCFG[%s] Opening up application file", p.appName)

	var cfgData []byte
	path := metakvAppsPath + p.appName

	// Adding sleep until source of MB-26702 is known
	time.Sleep(5 * time.Second)

	// Keeping metakv lookup in retry loop. There is potential metakv related race between routine that gets notified about updates
	// to metakv path and routine that does metakv lookup
	util.Retry(util.NewFixedBackoff(bucketOpRetryInterval), metakvGetCallback, p, path, &cfgData)

	config := cfg.GetRootAsConfig(cfgData, 0)

	p.app = new(common.AppConfig)
	p.app.AppCode = string(config.AppCode())
	p.app.AppName = string(config.AppName())
	p.app.AppState = fmt.Sprintf("%v", appUndeployed)
	p.app.AppVersion = util.GetHash(p.app.AppCode)
	p.app.LastDeploy = time.Now().UTC().Format("2006-01-02T15:04:05.000000000-0700")
	p.app.ID = int(config.Id())
	p.app.Settings = make(map[string]interface{})

	d := new(cfg.DepCfg)
	depcfg := config.DepCfg(d)

	var user, password string
	util.Retry(util.NewFixedBackoff(time.Second), getHTTPServiceAuth, p, &user, &password)
	p.auth = fmt.Sprintf("%s:%s", user, password)

	p.bucket = string(depcfg.SourceBucket())
	p.cfgData = string(cfgData)
	p.metadatabucket = string(depcfg.MetadataBucket())

	settingsPath := metakvAppSettingsPath + p.appName
	sData, sErr := util.MetakvGet(settingsPath)
	if sErr != nil {
		logging.Errorf("DCFG[%s] Failed to fetch settings from metakv, err: %v", p.appName, sErr)
		return sErr
	}

	settings := make(map[string]interface{})
	uErr := json.Unmarshal(sData, &settings)
	if uErr != nil {
		logging.Errorf("DCFG[%s] Failed to unmarshal settings received from metakv, err: %v", p.appName, uErr)
		return uErr
	}

	if val, ok := settings["cleanup_timers"]; !ok {
		p.cleanupTimers = false
	} else {
		p.cleanupTimers = val.(bool)
	}

	if val, ok := settings["dcp_stream_boundary"]; !ok {
		p.dcpStreamBoundary = common.DcpStreamBoundary("everything")
	} else {
		p.dcpStreamBoundary = common.DcpStreamBoundary(val.(string))
	}

	if val, ok := settings["log_level"]; !ok {
		p.logLevel = "INFO"
	} else {
		p.logLevel = val.(string)
	}

	if val, ok := settings["tick_duration"]; !ok {
		p.statsTickDuration = time.Duration(5000)
	} else {
		p.statsTickDuration = time.Duration(val.(float64))
	}

	if val, ok := settings["worker_count"]; !ok {
		p.workerCount = 3
	} else {
		p.workerCount = int(val.(float64))
	}

	if val, ok := settings["timer_worker_pool_size"]; !ok {
		p.timerWorkerPoolSize = 3
	} else {
		p.timerWorkerPoolSize = int(val.(float64))
	}

	if val, ok := settings["sock_batch_size"]; !ok {
		p.socketWriteBatchSize = 100
	} else {
		p.socketWriteBatchSize = int(val.(float64))
	}

	if val, ok := settings["skip_timer_threshold"]; !ok {
		p.skipTimerThreshold = 86400
	} else {
		p.skipTimerThreshold = int(val.(float64))
	}

	val, ok := settings["rbacuser"]
	if !ok {
		logging.Errorf("DCFG[%s] RBAC username not supplied", p.appName)
		return fmt.Errorf("rbac username missing")
	}
	p.rbacUser = val.(string)

	val, ok = settings["rbacpass"]
	if !ok {
		logging.Errorf("DCFG[%s] RBAC password not supplied", p.appName)
		return fmt.Errorf("rbac password missing")
	}
	p.rbacPass = val.(string)

	if val, ok := settings["lcb_inst_capacity"]; ok {
		p.lcbInstCapacity = int(val.(float64))
	} else {
		p.lcbInstCapacity = 5
	}

	if val, ok := settings["enable_recursive_mutation"]; ok {
		p.enableRecursiveMutation = val.(bool)
	} else {
		p.enableRecursiveMutation = false
	}

	if val, ok := settings["deadline_timeout"]; ok {
		p.socketTimeout = time.Duration(val.(float64)) * time.Second
	} else {
		p.socketTimeout = time.Duration(2 * time.Second)
	}

	if val, ok := settings["vb_ownership_giveup_routine_count"]; ok {
		p.vbOwnershipGiveUpRoutineCount = int(val.(float64))
	} else {
		p.vbOwnershipGiveUpRoutineCount = 3
	}

	if val, ok := settings["vb_ownership_takeover_routine_count"]; ok {
		p.vbOwnershipTakeoverRoutineCount = int(val.(float64))
	} else {
		p.vbOwnershipTakeoverRoutineCount = 3
	}

	if val, ok := settings["execution_timeout"]; ok {
		p.executionTimeout = int(val.(float64))
	} else {
		p.executionTimeout = 1
	}

	if val, ok := settings["fuzz_offset"]; ok {
		p.fuzzOffset = int(val.(float64))
	} else {
		p.fuzzOffset = 30
	}

	if val, ok := settings["cpp_worker_thread_count"]; ok {
		p.cppWorkerThrCount = int(val.(float64))
	} else {
		p.cppWorkerThrCount = 1
	}

	if val, ok := settings["memory_quota"]; ok {
		p.plasmaMemQuota = int64(val.(float64))
	} else {
		p.plasmaMemQuota = 256 // in MB
	}

	if val, ok := settings["xattr_doc_timer_entry_prune_threshold"]; ok {
		p.xattrEntryPruneThreshold = int(val.(float64))
	} else {
		p.xattrEntryPruneThreshold = 100
	}

	if val, ok := settings["app_log_dir"]; ok {
		os.MkdirAll(val.(string), 0755)
		p.appLogPath = fmt.Sprintf("%s/%s", val.(string), p.appName)
	} else {
		os.MkdirAll(p.eventingDir, 0755)
		p.appLogPath = fmt.Sprintf("%s/%s.log", p.eventingDir, p.appName)
	}

	if val, ok := settings["app_log_max_size"]; ok {
		p.appLogMaxSize = int64(val.(float64))
	} else {
		p.appLogMaxSize = 1024 * 1024 * 10
	}

	if val, ok := settings["app_log_max_files"]; ok {
		p.appLogMaxFiles = int(val.(float64))
	} else {
		p.appLogMaxFiles = 10
	}

	if val, ok := settings["curl_timeout"]; ok {
		p.curlTimeout = int64(val.(float64))
	} else {
		p.curlTimeout = int64(500)
	}

	if val, ok := settings["worker_queue_cap"]; ok {
		p.workerQueueCap = int64(val.(float64))
	} else {
		p.workerQueueCap = int64(1000 * 1000)
	}

	p.app.Settings = settings

	logLevel := settings["log_level"].(string)
	logging.SetLogLevel(util.GetLogLevel(logLevel))

	logging.Infof("DCFG[%s] Loaded app => wc: %v auth: %v bucket: %v statsTickD: %v",
		p.appName, p.workerCount, p.auth, p.bucket, p.statsTickDuration)

	if p.workerCount <= 0 {
		return fmt.Errorf("%v", errorUnexpectedWorkerCount)
	}

	hostaddr := fmt.Sprintf("127.0.0.1:%s", p.nsServerPort)

	localAddress, err := util.LocalEventingServiceHost(p.auth, hostaddr)
	if err != nil {
		logging.Errorf("DCFG[%s] Failed to get address for local eventing node, err :%v", p.appName, err)
		return err
	}

	p.kvHostPorts, err = util.KVNodesAddresses(p.auth, hostaddr)
	if err != nil {
		logging.Errorf("DCFG[%s] Failed to get list of kv nodes in the cluster, err: %v", p.appName, err)
		return err
	}

	p.nsServerHostPort = fmt.Sprintf("%s:%s", localAddress, p.nsServerPort)

	return nil
}
