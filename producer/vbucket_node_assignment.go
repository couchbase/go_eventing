package producer

import (
	"fmt"
	"sort"
	"time"

	"github.com/couchbase/eventing/util"
	"github.com/couchbase/indexing/secondary/common"
	"github.com/couchbase/indexing/secondary/logging"
)

// Generates the vbucket to eventing node assignment, ideally generated map should
// be consistent across all nodes
func (p *Producer) vbEventingNodeAssign() {

	util.Retry(util.NewFixedBackoff(time.Second), getKVNodesAddressesOpCallback, p)

	util.Retry(util.NewFixedBackoff(time.Second), getEventingNodesAddressesOpCallback, p)

	util.Retry(util.NewFixedBackoff(time.Second), getNsServerNodesAddressesOpCallback, p)

	eventingNodeAddrs := p.getEventingNodeAddrs()
	vbucketsPerNode := NumVbuckets / len(eventingNodeAddrs)
	var vbNo int
	var startVb uint16

	p.Lock()
	defer p.Unlock()
	p.vbEventingNodeAssignMap = make(map[uint16]string)

	vbCountPerNode := make([]int, len(eventingNodeAddrs))
	for i := 0; i < len(eventingNodeAddrs); i++ {
		vbCountPerNode[i] = vbucketsPerNode
		vbNo += vbucketsPerNode
	}

	remainingVbs := NumVbuckets - vbNo
	if remainingVbs > 0 {
		for i := 0; i < remainingVbs; i++ {
			vbCountPerNode[i] = vbCountPerNode[i] + 1
		}
	}

	for i, v := range vbCountPerNode {
		for j := 0; j < v; j++ {
			p.vbEventingNodeAssignMap[startVb] = eventingNodeAddrs[i]
			startVb++
		}
		fmt.Printf("eventing node index: %d\tstartVb: %d\n", i, startVb)
	}
}

func (p *Producer) initWorkerVbMap() {

	hostAddress := fmt.Sprintf("127.0.0.1:%s", p.NsServerPort)

	eventingNodeAddr, err := util.CurrentEventingNodeAddress(p.auth, hostAddress)
	if err != nil {
		logging.Errorf("PRDR[%s:%d] Failed to get address for current eventing node, err: %v", p.AppName, p.LenRunningConsumers(), err)
	}

	// vbuckets the current eventing node is responsible to handle
	var vbucketsToHandle []int

	for k, v := range p.vbEventingNodeAssignMap {
		if v == eventingNodeAddr {
			vbucketsToHandle = append(vbucketsToHandle, int(k))
		}
	}

	sort.Ints(vbucketsToHandle)

	logging.Infof("PRDR[%s:%d] eventingAddr: %v vbucketsToHandle, len: %d dump: %v", p.AppName, p.LenRunningConsumers(), eventingNodeAddr, len(vbucketsToHandle), vbucketsToHandle)

	vbucketPerWorker := len(vbucketsToHandle) / p.workerCount
	var startVbIndex int

	vbCountPerWorker := make([]int, p.workerCount)
	for i := 0; i < p.workerCount; i++ {
		vbCountPerWorker[i] = vbucketPerWorker
		startVbIndex += vbucketPerWorker
	}

	remainingVbs := len(vbucketsToHandle) - startVbIndex
	if remainingVbs > 0 {
		for i := 0; i < remainingVbs; i++ {
			vbCountPerWorker[i] = vbCountPerWorker[i] + 1
		}
	}

	p.Lock()
	defer p.Unlock()

	var workerName string
	p.workerVbucketMap = make(map[string][]uint16)

	startVbIndex = 0

	for i := 0; i < p.workerCount; i++ {
		workerName = fmt.Sprintf("worker_%s_%d", p.app.AppName, i)

		for j := 0; j < vbCountPerWorker[i]; j++ {
			p.workerVbucketMap[workerName] = append(p.workerVbucketMap[workerName], uint16(vbucketsToHandle[startVbIndex]))
			startVbIndex++
		}
	}

}

func (p *Producer) getKvVbMap() {

	var cinfo *common.ClusterInfoCache

	util.Retry(util.NewFixedBackoff(time.Second), getClusterInfoCacheOpCallback, p, &cinfo)

	kvAddrs := cinfo.GetNodesByServiceType(DataService)

	p.kvVbMap = make(map[uint16]string)

	for _, kvaddr := range kvAddrs {
		addr, err := cinfo.GetServiceAddress(kvaddr, DataService)
		if err != nil {
			logging.Errorf("VBNA[%s:%d] Failed to get address of KV host, err: %v", p.AppName, p.LenRunningConsumers(), err)
			continue
		}

		vbs, err := cinfo.GetVBuckets(kvaddr, "default")
		if err != nil {
			logging.Errorf("VBNA[%s:%d] Failed to get vbuckets for given kv common.NodeId, err: %v", p.AppName, p.LenRunningConsumers(), err)
			continue
		}

		for i := 0; i < len(vbs); i++ {
			p.kvVbMap[uint16(vbs[i])] = addr
		}
	}
}
