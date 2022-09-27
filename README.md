[![Go](https://github.com/vegaprotocol/vega-exporter/actions/workflows/go.yml/badge.svg)](https://github.com/vegaprotocol/vega-exporter/actions/workflows/go.yml)

# VEGA Prometheus Exporter

Export useful metrics for Vega Protocol

## Exported metrics
```
vega_withdrawals_sum_total{chain_id, asset, status, eth_tx}
vega_withdrawals_count_total{chain_id, asset, status, eth_tx}

vega_transfers_sum_total{chain_id, asset, status}
vega_transfers_count_total{chain_id, asset, status}

vega_asset_quantum{chain_id, asset}
vega_asset_decimals{chain_id, asset}

vega_market_best_bid_price{chain_id, market}
vega_market_best_offer_price{chain_id, market}
```