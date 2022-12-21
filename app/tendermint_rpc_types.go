package app

import "time"

type ValidatorsResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		BlockHeight string `json:"block_height"`
		Validators  []struct {
			Address string `json:"address"`
			PubKey  struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"pub_key"`
			VotingPower      string `json:"voting_power"`
			ProposerPriority string `json:"proposer_priority"`
		} `json:"validators"`
		Count string `json:"count"`
		Total string `json:"total"`
	} `json:"result"`
}

type TmEvent struct {
	Query string `json:"query"`
	Data  struct {
		Type string `json:"type"`
	} `json:"data"`
	Event struct {
		CommandType []string `json:"command.type"`
		EventType   []string `json:"tm.event"`
		Submitter   []string `json:"tx.submitter"`
	} `json:"events"`
}

type BlockEvent struct {
	Query string `json:"query"`
	Data  struct {
		Type  string `json:"type"`
		Value struct {
			Block struct {
				Header struct {
					Version struct {
						Block string `json:"block"`
						App   string `json:"app"`
					} `json:"version"`
					ChainID     string    `json:"chain_id"`
					Height      string    `json:"height"`
					Time        time.Time `json:"time"`
					LastBlockID struct {
						Hash  string `json:"hash"`
						Parts struct {
							Total int    `json:"total"`
							Hash  string `json:"hash"`
						} `json:"parts"`
					} `json:"last_block_id"`
					LastCommitHash     string `json:"last_commit_hash"`
					DataHash           string `json:"data_hash"`
					ValidatorsHash     string `json:"validators_hash"`
					NextValidatorsHash string `json:"next_validators_hash"`
					ConsensusHash      string `json:"consensus_hash"`
					AppHash            string `json:"app_hash"`
					LastResultsHash    string `json:"last_results_hash"`
					EvidenceHash       string `json:"evidence_hash"`
					ProposerAddress    string `json:"proposer_address"`
				} `json:"header"`
				Data struct {
					Txs []string `json:"txs"`
				} `json:"data"`
				Evidence struct {
					Evidence []interface{} `json:"evidence"`
				} `json:"evidence"`
				LastCommit struct {
					Height  string `json:"height"`
					Round   int    `json:"round"`
					BlockID struct {
						Hash  string `json:"hash"`
						Parts struct {
							Total int    `json:"total"`
							Hash  string `json:"hash"`
						} `json:"parts"`
					} `json:"block_id"`
					Signatures []struct {
						BlockIDFlag      int       `json:"block_id_flag"`
						ValidatorAddress string    `json:"validator_address"`
						Timestamp        time.Time `json:"timestamp"`
						Signature        string    `json:"signature"`
					} `json:"signatures"`
				} `json:"last_commit"`
			} `json:"block"`
			ResultBeginBlock struct {
			} `json:"result_begin_block"`
			ResultEndBlock struct {
				ValidatorUpdates interface{} `json:"validator_updates"`
			} `json:"result_end_block"`
		} `json:"value"`
	} `json:"data"`
	Events struct {
		TmEvent []string `json:"tm.event"`
	} `json:"events"`
}
