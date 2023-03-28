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
	Jsonrpc string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Query string `json:"query"`
		Data  struct {
			Type  string `json:"type"`
			Value struct {
				TxResult struct {
					Height string `json:"height"`
					Index  int    `json:"index"`
					Tx     string `json:"tx"`
					Result struct {
						Events []struct {
							Type       string `json:"type"`
							Attributes []struct {
								Key   string `json:"key"`
								Value string `json:"value"`
								Index bool   `json:"index"`
							} `json:"attributes"`
						} `json:"events"`
					} `json:"result"`
				} `json:"TxResult"`
			} `json:"value"`
		} `json:"data"`
		Events struct {
			TxHash           []string `json:"tx.hash"`
			TxHeight         []string `json:"tx.height"`
			TxSubmitter      []string `json:"tx.submitter"`
			CommandType      []string `json:"command.type"`
			CommandMarket    []string `json:"command.market"`
			CommandReference []string `json:"command.reference"`
			TmEvent          []string `json:"tm.event"`
		} `json:"events"`
	} `json:"result"`
}

type TmStatusResp struct {
	NodeInfo struct {
		ProtocolVersion struct {
			P2P   string `json:"p2p"`
			Block string `json:"block"`
			App   string `json:"app"`
		} `json:"protocol_version"`
		ID         string `json:"id"`
		ListenAddr string `json:"listen_addr"`
		Network    string `json:"network"`
		Version    string `json:"version"`
		Channels   string `json:"channels"`
		Moniker    string `json:"moniker"`
		Other      struct {
			TxIndex    string `json:"tx_index"`
			RPCAddress string `json:"rpc_address"`
		} `json:"other"`
	} `json:"node_info"`
	SyncInfo struct {
		LatestBlockHash     string    `json:"latest_block_hash"`
		LatestAppHash       string    `json:"latest_app_hash"`
		LatestBlockHeight   string    `json:"latest_block_height"`
		LatestBlockTime     time.Time `json:"latest_block_time"`
		EarliestBlockHash   string    `json:"earliest_block_hash"`
		EarliestAppHash     string    `json:"earliest_app_hash"`
		EarliestBlockHeight string    `json:"earliest_block_height"`
		EarliestBlockTime   time.Time `json:"earliest_block_time"`
		CatchingUp          bool      `json:"catching_up"`
	} `json:"sync_info"`
	ValidatorInfo struct {
		Address string `json:"address"`
		PubKey  struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"pub_key"`
		VotingPower string `json:"voting_power"`
	} `json:"validator_info"`
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
