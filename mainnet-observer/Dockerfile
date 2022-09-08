FROM alpine:3.16

# Config
ENV NETWORK=mainnet1
ENV ETH_NETWORK=mainnet
ENV UID=1000

ENV VEGA_VERSION=0.53.0
ENV DATA_NODE_VERSION=0.53.1
ENV INFURA_TOKEN=8c026e3c74c9488f844b1096d5aa903b
ENV PASSPHRASE=super_secret
ENV PASSPHRASE_FILE=/vega/passphrase
ENV WALLET_NAME=my_wallet
ENV INFURA_API_ADDRESS=https://${ETH_NETWORK}.infura.io/v3/$INFURA_TOKEN

WORKDIR /vega
ENV TMHOME=/vega/tm
ENV VEGAHOME=/vega/vega
ENV VEGA_DATA_NODE_CONFIG=/vega/data-node
ENV PATH=/vega/bin:$PATH

# Install tools
RUN apk add --no-cache unzip=6.0-r9

# Make a vega user
RUN mkdir /vega/bin &&\
    addgroup -S vega --gid $UID &&\
    adduser -S vega -G vega --uid $UID &&\
    chown -R vega /vega
USER vega

# Get fetch for getting releases from github
RUN wget -q "https://github.com/gruntwork-io/fetch/releases/download/v0.4.2/fetch_linux_amd64" -O bin/fetch &&\
    chmod +x bin/fetch

# Get dasel, for editing TOML config files
RUN wget -q https://github.com/TomWright/dasel/releases/download/v1.22.1/dasel_linux_amd64 -O bin/dasel && \
    chmod +x bin/dasel

# Get vega node
RUN rm -f bin/vega-linux-amd64 && \
    rm -f bin/vega && \
    bin/fetch --repo="https://github.com/vegaprotocol/vega" --tag="v${VEGA_VERSION}" --release-asset="vega-linux-amd64" ./bin && \
    cp bin/vega-linux-amd64 bin/vega && \
    chmod +x bin/vega && \
    ./bin/vega version

# Get data node
RUN rm -f bin/data-node-linux-amd64 && \
    rm -f bin/data-node && \
    bin/fetch --repo="https://github.com/vegaprotocol/data-node" --tag="v${DATA_NODE_VERSION}" --release-asset="data-node-linux-amd64" ./bin && \
    cp bin/data-node-linux-amd64 bin/data-node &&\
    chmod +x bin/data-node && \
    ./bin/data-node version

# Set up tendermint
COPY peers_${NETWORK} peers
RUN set -o pipefail &&\
    vega tm init &&\
    wget --no-verbose https://raw.githubusercontent.com/vegaprotocol/networks/master/$NETWORK/genesis.json -O $TMHOME/config/genesis.json &&\
    dasel put string -f $TMHOME/config/config.toml p2p.persistent_peers "$(cut -f2 peers|paste -s -d,)" && \
    dasel put string -f $TMHOME/config/config.toml proxy_app tcp://localhost:26658 && \
    dasel put string -f $TMHOME/config/config.toml rpc.laddr tcp://0.0.0.0:26657 && \
    rm peers

# Set up vega
RUN echo $PASSPHRASE > $PASSPHRASE_FILE &&\
    vega init --home=$VEGAHOME --output=json --nodewallet-passphrase-file=$PASSPHRASE_FILE full &&\
    dasel put string -f $VEGAHOME/config/node/config.toml NodeWallet.ETH.Address $INFURA_API_ADDRESS &&\
    dasel put bool -f $VEGAHOME/config/node/config.toml Broker.Socket.Enabled true && \
    dasel put string -f $VEGAHOME/config/node/config.toml Broker.Socket.Address localhost &&\
    dasel put string -f $VEGAHOME/config/node/config.toml Blockchain.Tendermint.ClientAddr tcp://localhost:26657 &&\
    dasel put string -f $VEGAHOME/config/node/config.toml Blockchain.Tendermint.ServerAddr 0.0.0.0 &&\
    dasel put string -f $VEGAHOME/config/node/config.toml Snapshot.Level Debug

# Set up wallets
RUN vega nodewallet generate --home=$VEGAHOME --output=json -c vega -p $PASSPHRASE_FILE --wallet-passphrase-file=$PASSPHRASE_FILE &&\
    vega nodewallet generate --home=$VEGAHOME --output=json -c ethereum -p $PASSPHRASE_FILE --wallet-passphrase-file=$PASSPHRASE_FILE

# Set up data-node
RUN data-node init --home=$VEGAHOME && \
    dasel put string -f $VEGAHOME/config/data-node/config.toml API.CoreNodeIP localhost
