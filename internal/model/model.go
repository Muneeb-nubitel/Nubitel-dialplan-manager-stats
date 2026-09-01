package model

type QueueMemberStatsRequest struct {
	CampaignIDs []string `json:"campaign_ids"`
}

type LiveQueueMember struct {
	CallUUID       string `json:"callUuid"`
	CampaignID     string `json:"campaign_id,omitempty"`
	Queue          string `json:"queue"`
	QueueExtension string `json:"queue_extension"`
	Domain         string `json:"domain"`
	InstanceID     string `json:"instance_id"`
	CallerNumber   string `json:"cid_number,omitempty"`
	CallerName     string `json:"cid_name,omitempty"`
	JoinedEpoch    int64  `json:"joined_epoch"`
	RejoinedEpoch  int64  `json:"rejoined_epoch"`
	Priority       int    `json:"priority"`
	ServingAgent   string `json:"serving_agent"`
	AgentLegUUID   string `json:"session_uuid"`
	State          string `json:"state"`
	UpdatedAt      string `json:"updatedAt"`
}

type QueueMemberStatsResponse struct {
	DomainName  string            `json:"domainName"`
	CampaignIDs []string          `json:"campaignIds"`
	Total       int               `json:"total"`
	Records     []LiveQueueMember `json:"records"`
}

type AgentDashboard struct {
	UUID       string `json:"uuid"`
	AgentID    int    `json:"agent_id"`
	DomainName string `json:"domain_name"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	CallStatus string `json:"call_status"`
}

type AgentDashboardResponse struct {
	UUID    string `json:"uuid"`
	AgentID int    `json:"agent_id"`
	Domain  string `json:"domain"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	State   string `json:"state"`
}

type AgentDashboardListResponse struct {
	Agents []AgentDashboardResponse `json:"agents"`
}
