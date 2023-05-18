package token

import (
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

func GetERC20Balance(erc20TokenAddr, assetPoolAddr, ethereumRpcAddr string) (string, float64, error) {
	client, err := ethclient.Dial(ethereumRpcAddr)
	if err != nil {
		return "", 0, err
	}

	// VEGA erc20 contract Address
	tokenAddress := common.HexToAddress(erc20TokenAddr)
	instance, err := NewToken(tokenAddress, client)
	if err != nil {
		return "", 0, err
	}

	address := common.HexToAddress(assetPoolAddr)
	bal, err := instance.BalanceOf(&bind.CallOpts{}, address)
	if err != nil {
		return "", 0, err
	}

	decimals, err := instance.Decimals(&bind.CallOpts{})
	if err != nil {
		return "", 0, err
	}

	name, err := instance.Symbol(&bind.CallOpts{})
	if err != nil {
		return "", 0, err
	}

	fbal := new(big.Float)
	fbal.SetString(bal.String())
	value := new(big.Float).Quo(fbal, big.NewFloat(math.Pow10(int(decimals))))

	balance, _ := value.Float64()
	return name, balance, nil
}
