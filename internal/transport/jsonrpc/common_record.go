package jsonrpc

import (
	"context"
	"sort"
	"time"

	"github.com/sonar-probe/sonar/internal/features/ping"
	"github.com/sonar-probe/sonar/internal/platform/clients"
	"github.com/sonar-probe/sonar/internal/platform/models"
	"github.com/sonar-probe/sonar/internal/platform/recordquery"
	"github.com/sonar-probe/sonar/pkg/downsample"
	"github.com/sonar-probe/sonar/pkg/rpc"
)

func init() {
	Register("getRecords", getRecords)
}

func getRecords(ctx context.Context, req *rpc.JsonRpcRequest) (any, *rpc.JsonRpcError) {
	meta := rpc.MetaFromContext(ctx)
	var params struct {
		Type     string     `json:"type"`      // "load" | "ping"; default "load"
		UUID     string     `json:"uuid"`      // client uuid; empty = all clients
		Hours    int        `json:"hours"`     // time window in hours; default 1 if start/end not provided
		Start    *time.Time `json:"start"`     // RFC3339 with an explicit timezone (optional)
		End      *time.Time `json:"end"`       // RFC3339 with an explicit timezone (optional)
		LoadType string     `json:"load_type"` // for type=load: cpu|gpu|ram|swap|load|temp|disk|network|process|connections|all
		TaskID   int        `json:"task_id"`   // for type=ping: optional task id; -1 or omitted means all
		MaxCount int        `json:"maxCount"`  // max number of points; -1 unlimited; default 4000
	}
	if err := req.BindParams(&params); err != nil {
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid request body: "+err.Error(), nil)
	}

	// defaults
	if params.Type == "" {
		params.Type = "load"
	}
	// parse time window
	var startTime, endTime time.Time
	if params.Start != nil || params.End != nil {
		// allow partial: missing end means now
		if params.End == nil {
			endTime = time.Now().UTC()
		} else {
			endTime = params.End.UTC()
		}
		if params.Start == nil {
			// default to 1 hour before end
			startTime = endTime.Add(-1 * time.Hour)
		} else {
			startTime = params.Start.UTC()
		}
	} else {
		hours := params.Hours
		if hours <= 0 {
			hours = 1 // default 1 hour
		}
		endTime = time.Now().UTC()
		startTime = endTime.Add(-time.Duration(hours) * time.Hour)
	}

	// Hidden filtering for non-admin
	isAdmin := meta.Principal != nil && meta.Principal.HasRole(rpc.RoleAdmin)
	hidden := map[string]bool{}
	if !isAdmin {
		cinfo, err := clients.GetAllClientBasicInfo()
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to get client info", err.Error())
		}
		for _, c := range cinfo {
			if c.Hidden {
				hidden[c.UUID] = true
			}
		}
		if params.UUID != "" && hidden[params.UUID] {
			return nil, rpc.MakeError(rpc.InvalidParams, "UUID not found", params.UUID)
		}
	}

	switch params.Type {
	case "load":
		// fetch load records
		recs, err := getLoadRecordsCombined(params.UUID, startTime, endTime)
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to fetch records", err.Error())
		}
		// hidden filter on non-admin
		if !isAdmin {
			filtered := recs[:0]
			for _, r := range recs {
				if hidden[r.Client] {
					continue
				}
				filtered = append(filtered, r)
			}
			recs = filtered
		}

		// resolve maxCount default for load
		maxCount := params.MaxCount
		if maxCount == 0 {
			maxCount = 4000
		}

		// optional load_type filtering -> group by client
		if params.LoadType != "" && params.LoadType != "all" {
			items := filterRecordsByLoadType(recs, params.LoadType)
			grouped := make(map[string][]flatRecord)
			for _, it := range items {
				grouped[it.Client] = append(grouped[it.Client], it)
			}
			// sort and count
			total := 0
			groupsMeta := make([]downsample.AllocationGroup[string], 0, len(grouped))
			for name := range grouped {
				arr := grouped[name]
				sort.Slice(arr, func(i, j int) bool { return arr[i].Time.Before(arr[j].Time) })
				grouped[name] = arr
				l := len(arr)
				total += l
				groupsMeta = append(groupsMeta, downsample.AllocationGroup[string]{Key: name, Length: l})
			}
			// downsample across all clients proportionally
			if maxCount != -1 && total > maxCount {
				targets := downsample.AllocateTargets(groupsMeta, maxCount)
				total = 0
				for name, k := range targets {
					grouped[name] = downsample.SampleEvenly(grouped[name], k)
					total += len(grouped[name])
				}
			}
			return struct {
				Count    int                     `json:"count"`
				Records  map[string][]flatRecord `json:"records"`
				LoadType string                  `json:"load_type"`
				From     time.Time               `json:"from"`
				To       time.Time               `json:"to"`
			}{Count: total, Records: grouped, LoadType: params.LoadType, From: startTime.UTC(), To: endTime.UTC()}, nil
		}
		// default: return full records, grouped by client
		grouped := make(map[string][]models.Record)
		for _, r := range recs {
			grouped[r.Client] = append(grouped[r.Client], r)
		}
		total := 0
		groupsMeta := make([]downsample.AllocationGroup[string], 0, len(grouped))
		for name := range grouped {
			arr := grouped[name]
			sort.Slice(arr, func(i, j int) bool { return arr[i].Time.Before(arr[j].Time) })
			grouped[name] = arr
			l := len(arr)
			total += l
			groupsMeta = append(groupsMeta, downsample.AllocationGroup[string]{Key: name, Length: l})
		}
		if maxCount != -1 && total > maxCount {
			targets := downsample.AllocateTargets(groupsMeta, maxCount)
			total = 0
			for name, k := range targets {
				grouped[name] = downsample.SampleEvenly(grouped[name], k)
				total += len(grouped[name])
			}
		}
		return struct {
			Count   int                        `json:"count"`
			Records map[string][]models.Record `json:"records"`
			From    time.Time                  `json:"from"`
			To      time.Time                  `json:"to"`
		}{Count: total, Records: grouped, From: startTime.UTC(), To: endTime.UTC()}, nil

	case "ping":
		taskID := params.TaskID
		if taskID == 0 {
			taskID = -1
		}
		recs, err := ping.GetPingRecords(params.UUID, taskID, startTime, endTime)
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to fetch ping records", err.Error())
		}
		// hidden filter
		if !isAdmin {
			filtered := recs[:0]
			for _, r := range recs {
				if r.Client != "" && hidden[r.Client] {
					continue
				}
				filtered = append(filtered, r)
			}
			recs = filtered
		}

		type RecordsResp struct {
			TaskID uint      `json:"task_id,omitempty"`
			Time   time.Time `json:"time"`
			Value  int       `json:"value"`
			Client string    `json:"client,omitempty"`
		}
		type ClientBasicInfo struct {
			Client string  `json:"client"`
			Loss   float64 `json:"loss"`
			Min    int     `json:"min"`
			Max    int     `json:"max"`
		}
		type Resp struct {
			Count     int               `json:"count"`
			BasicInfo []ClientBasicInfo `json:"basic_info,omitempty"`
			Records   []RecordsResp     `json:"records"`
			Tasks     []map[string]any  `json:"tasks"`
			From      time.Time         `json:"from"`
			To        time.Time         `json:"to"`
		}

		response := &Resp{Count: 0, Records: []RecordsResp{}, From: startTime.UTC(), To: endTime.UTC()}

		// stats per client
		clientStats := make(map[string]struct {
			total int
			loss  int
			min   int
			max   int
		})

		for _, r := range recs {
			rr := RecordsResp{
				TaskID: r.TaskID,
				Time:   r.Time,
				Value:  r.Value,
				Client: r.Client,
			}
			st := clientStats[r.Client]
			st.total++
			if r.Value < 0 {
				st.loss++
			} else {
				if st.min == 0 || r.Value < st.min {
					st.min = r.Value
				}
				if r.Value > st.max {
					st.max = r.Value
				}
			}
			clientStats[r.Client] = st
			response.Records = append(response.Records, rr)
		}

		if len(clientStats) > 0 {
			response.BasicInfo = make([]ClientBasicInfo, 0, len(clientStats))
			for client, st := range clientStats {
				if client != "" && !isAdmin && hidden[client] {
					continue
				}
				loss := float64(0)
				if st.total > 0 {
					loss = float64(st.loss) / float64(st.total) * 100
				}
				response.BasicInfo = append(response.BasicInfo, ClientBasicInfo{
					Client: client,
					Loss:   loss,
					Min:    st.min,
					Max:    st.max,
				})
			}
		}

		// tasks summary (always included for ping type; do not expose target field)
		pingTasks, err := ping.GetAllPingTasks()
		if err != nil {
			return nil, rpc.MakeError(rpc.InternalError, "Failed to fetch ping tasks", err.Error())
		}
		toList := make([]map[string]any, 0, len(pingTasks))
		for _, t := range pingTasks {
			if taskID != -1 && t.ID != uint(taskID) {
				continue
			}
			if params.UUID != "" { // ensure task assigned to specific client when filtering by uuid
				if !t.AppliesToClient(params.UUID) {
					continue
				}
			}
			total := 0
			lossCount := 0
			minLat := 0
			maxLat := 0
			sum := 0
			valid := 0
			latestVal := -1
			var latestTS time.Time
			// 收集该任务的所有有效(非丢包)延迟值以计算百分位
			latencies := make([]int, 0, 64)
			for _, r := range recs {
				if r.TaskID != t.ID {
					continue
				}
				if params.UUID != "" && r.Client != params.UUID {
					continue
				}
				total++
				if r.Value < 0 { // 丢包
					lossCount++
					continue
				}
				valid++
				sum += r.Value
				latencies = append(latencies, r.Value)
				if minLat == 0 || r.Value < minLat {
					minLat = r.Value
				}
				if r.Value > maxLat {
					maxLat = r.Value
				}
				// track latest non-negative value
				ts := r.Time
				if latestTS.IsZero() || ts.After(latestTS) {
					latestTS = ts
					latestVal = r.Value
				}
			}

			// 计算 P50 / P99
			p50 := 0
			p99 := 0
			if len(latencies) > 0 {
				sort.Ints(latencies)
				p50, p99 = ping.PercentileLatencies(latencies)
			}
			ratio := 0.0
			if len(latencies) >= ping.MinSamplesForVolatility {
				ratio, _ = ping.Volatility(float64(p50), float64(p99))
			}
			lossRate := 0.0
			if total > 0 {
				lossRate = float64(lossCount) / float64(total) * 100
			}
			avg := 0
			if valid > 0 {
				avg = sum / valid
			}
			info := map[string]any{
				"id":            t.ID,
				"name":          t.Name,
				"type":          t.Type,
				"interval":      t.Interval,
				"default_on":    t.DefaultOn,
				"loss":          lossRate,
				"min":           minLat,
				"max":           maxLat,
				"avg":           avg,
				"latest":        latestVal,
				"total":         total,
				"p50":           p50,
				"p99":           p99,
				"p99_p50_ratio": ratio,
			}
			if params.UUID == "" && taskID != -1 { // retain existing behavior of exposing clients only when filtering by task
				info["clients"] = t.Clients
			}
			toList = append(toList, info)
		}
		response.Tasks = toList
		// apply maxCount for ping
		maxCount := params.MaxCount
		if maxCount == 0 {
			maxCount = 4000
		}
		if maxCount != -1 && len(response.Records) > maxCount {
			// group records by TaskID for proportional downsampling
			taskGroups := make(map[uint][]RecordsResp)
			for _, r := range response.Records {
				taskGroups[r.TaskID] = append(taskGroups[r.TaskID], r)
			}

			// sort each group by time
			for taskID := range taskGroups {
				sort.Slice(taskGroups[taskID], func(i, j int) bool {
					return taskGroups[taskID][i].Time.Before(taskGroups[taskID][j].Time)
				})
			}

			groupsMeta := make([]downsample.AllocationGroup[uint], 0, len(taskGroups))
			for taskID, records := range taskGroups {
				groupsMeta = append(groupsMeta, downsample.AllocationGroup[uint]{
					Key:    taskID,
					Length: len(records),
				})
			}
			targets := downsample.AllocateTargets(groupsMeta, maxCount)

			// downsample each task group
			downsampledRecords := make([]RecordsResp, 0, maxCount)
			for taskID, records := range taskGroups {
				targetCount := targets[taskID]
				sampled := downsample.SampleEvenly(records, targetCount)
				downsampledRecords = append(downsampledRecords, sampled...)
			}

			response.Records = downsampledRecords
		}
		response.Count = len(response.Records)
		// sort by time asc
		sort.Slice(response.Records, func(i, j int) bool {
			return response.Records[i].Time.Before(response.Records[j].Time)
		})
		return response, nil
	default:
		return nil, rpc.MakeError(rpc.InvalidParams, "Invalid type, expected 'load' or 'ping'", params.Type)
	}
}

// ---------- helpers for load records ----------

// getLoadRecordsCombined fetches records for a client or all clients within a time range,
// combining recent short-term table and long-term table with 15-min grouping for recent part.
func getLoadRecordsCombined(uuid string, start, end time.Time) ([]models.Record, error) {
	// prefer the existing function when uuid provided
	if uuid != "" {
		return recordquery.GetRecordsByClientAndTime(uuid, start, end)
	}
	// 所有客户端：统一通过 records 包查询，启用 metric store 时自动走 metric store
	return recordquery.GetRecordsByTime(start, end)
}

// flatRecord is a projection used when load_type is specified.
type flatRecord struct {
	Client         string    `json:"client"`
	Time           time.Time `json:"time"`
	CPU            *float32  `json:"cpu,omitempty"`
	GPU            *float32  `json:"gpu,omitempty"`
	RAM            *int64    `json:"ram,omitempty"`
	RAMTotal       *int64    `json:"ram_total,omitempty"`
	Swap           *int64    `json:"swap,omitempty"`
	SwapTotal      *int64    `json:"swap_total,omitempty"`
	Load           *float32  `json:"load,omitempty"`
	Temp           *float32  `json:"temp,omitempty"`
	Disk           *int64    `json:"disk,omitempty"`
	DiskTotal      *int64    `json:"disk_total,omitempty"`
	NetIn          *int64    `json:"net_in,omitempty"`
	NetOut         *int64    `json:"net_out,omitempty"`
	NetTotalUp     *int64    `json:"net_total_up,omitempty"`
	NetTotalDown   *int64    `json:"net_total_down,omitempty"`
	Process        *int      `json:"process,omitempty"`
	Connections    *int      `json:"connections,omitempty"`
	ConnectionsUDP *int      `json:"connections_udp,omitempty"`
	Uptime         *int64    `json:"uptime,omitempty"`
}

func filterRecordsByLoadType(recs []models.Record, loadType string) []flatRecord {
	out := make([]flatRecord, 0, len(recs))
	for _, r := range recs {
		fr := flatRecord{Client: r.Client, Time: r.Time}
		switch loadType {
		case "cpu":
			v := r.CPU
			fr.CPU = &v
		case "gpu":
			v := r.GPU
			fr.GPU = &v
		case "ram":
			v := r.RAM
			fr.RAM = &v
			vt := r.RAMTotal
			fr.RAMTotal = &vt
		case "swap":
			v := r.Swap
			fr.Swap = &v
			vt := r.SwapTotal
			fr.SwapTotal = &vt
		case "load":
			v := r.Load
			fr.Load = &v
		case "temp":
			v := r.Temp
			fr.Temp = &v
		case "disk":
			v := r.Disk
			fr.Disk = &v
			vt := r.DiskTotal
			fr.DiskTotal = &vt
		case "network":
			vi := r.NetIn
			vo := r.NetOut
			vtu := r.NetTotalUp
			vtd := r.NetTotalDown
			fr.NetIn = &vi
			fr.NetOut = &vo
			fr.NetTotalUp = &vtu
			fr.NetTotalDown = &vtd
		case "process":
			v := r.Process
			fr.Process = &v
		case "connections":
			v := r.Connections
			fr.Connections = &v
			vu := r.ConnectionsUDP
			fr.ConnectionsUDP = &vu
		default:
			// unknown type: fallback to all fields as a full record would be returned elsewhere
			v := r.CPU
			fr.CPU = &v
		}
		out = append(out, fr)
	}
	return out
}
