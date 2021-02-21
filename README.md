# Bscdot

## Introduction

​		Bscdot is a cross-chain project based on ChainBridge developed by the ChainX team. In order to achieve two-way cross-chain, chainbridge needs to deploy a pallet on the Substrate chain that is equivalent to the smart contract in the EVM, so it cannot be deployed on polkadot. Our team has improved this. Through Bscdot, it can be passed without pallet. The multi-signature module realizes a completely decentralized token transfer across Polkadot, transferring Dot on Polkadot to Binance Smart Chain, and it can also be applied to kusama, chainX and other networks that have huge value but cannot deploy pallets on their own.

## Demo Video

https://www.youtube.com/watch?v=vTMIlM2oaJc&feature=youtu.be

## UI

https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-1.png

![b-1](https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-1.png)

![2](https://cdn.jsdelivr.net/gh/rjman-self/resources@master/images/bscdot-2.png)

## Contract address on testnet

```json
"opts": {
    "bridge": "0x8CFa80a07a39aD53F997CAf7b4F27863912acc86",
    "erc20Handler": "0x5bC3f2060eb6aC17a3EF34D33160451789667fB7",
    "erc721Handler": "0xa393Ed01637d693E4C19aA9a14e966b7adc5fE4D",
    "genericHandler": " 0x665DdC1a1722576554cAb2E471f370bD61dD230e",
}
```

## Running Locally

### Prerequisites

- bscdot binary
- solidity contract
- Polkadot JS Portal
- cb-sol-cli

### Deploy Contracts

To deploy the contracts on to the BSC chain

```
bscdot-sol-cli deploy --all --relayerThreshold 1
```

After running, the expected output looks like this:

```
================================================================
Url:        http://localhost:8545
Deployer:   0xff93B45308FD417dF303D6515aB04D9e89a750Ca
Gas Limit:   8000000
Gas Price:   20000000
Deploy Cost: 0.0

Options
=======
Chain Id:    0
Threshold:   2
Relayers:    0xff93B45308FD417dF303D6515aB04D9e89a750Ca,0x8e0a907331554AF72563Bd8D43051C2E64Be5d35,0x24962717f8fA5BA3b931bACaF9ac03924EB475a0,0x148FfB2074A9e59eD58142822b3eB3fcBffb0cd7,0x4CEEf6139f00F9F4535Ad19640Ff7A0137708485
Bridge Fee:  0
Expiry:      100

Contract Addresses
================================================================
Bridge:             0x62877dDCd49aD22f5eDfc6ac108e9a4b5D2bD88B
----------------------------------------------------------------
BEP20 Handler:      0x3167776db165D8eA0f51790CA2bbf44Db5105ADF
----------------------------------------------------------------
BEP721 Handler:     0x3f709398808af36ADBA86ACC617FeB7F5B7B193E
----------------------------------------------------------------
Generic Handler:    0x2B6Ab4b880A45a07d83Cf4d664Df4Ab85705Bc07
----------------------------------------------------------------
BEP20:              0x21605f71845f372A9ed84253d2D024B7B10999f4
----------------------------------------------------------------
BEP721:             0xd7E33e1bbf65dC001A0Eb1552613106CD7e40C31
----------------------------------------------------------------
Centrifuge Asset:   0xc279648CE5cAa25B9bA753dAb0Dfef44A069BaF4
================================================================
```

### Register Resources

```
# Register fungible resource ID with erc20 contract
bscdot-sol-cli bridge register-resource --resourceId "0x000000000000000000000000000000c76ebe4a02bbc34786d860b355f5a5ce00" --targetContract "0x21605f71845f372A9ed84253d2D024B7B10999f4"

# Register non-fungible resource ID with erc721 contract
bscdot-sol-cli bridge register-resource --resourceId "0x000000000000000000000000000000e389d61c11e5fe32ec1735b3cd38c69501" --targetContract "0xd7E33e1bbf65dC001A0Eb1552613106CD7e40C31" --handler "0x3f709398808af36ADBA86ACC617FeB7F5B7B193E"

# Register generic resource ID
bscdot-sol-cli bridge register-generic-resource --resourceId "0x000000000000000000000000000000f44be64d2de895454c3467021928e55e01" --targetContract "0xc279648CE5cAa25B9bA753dAb0Dfef44A069BaF4" --handler "0x2B6Ab4b880A45a07d83Cf4d664Df4Ab85705Bc07" --hash --deposit "" --execute "store(bytes32)"
```

### Running A Relayer

Here is an example config file for a single relayer ("Alice") using the contracts we've deployed.

```
{
  "chains": [
    {
      "name": "bsc",
      "type": "binance smart chain",
      "id": "0",
      "endpoint": "ws://localhost:8545",
      "from": "0xff93B45308FD417dF303D6515aB04D9e89a750Ca",
      "opts": {
        "bridge": "0x62877dDCd49aD22f5eDfc6ac108e9a4b5D2bD88B",
        "erc20Handler": "0x3167776db165D8eA0f51790CA2bbf44Db5105ADF",
        "erc721Handler": "0x3f709398808af36ADBA86ACC617FeB7F5B7B193E",
        "genericHandler": "0x2B6Ab4b880A45a07d83Cf4d664Df4Ab85705Bc07",
        "gasLimit": "1000000",
        "maxGasPrice": "20000000"
      }
    },
    {
      "name": "polkadot",
      "type": "substrate",
      "id": "1",
      "endpoint": "ws://localhost:9944",
      "from": "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
      "opts": {}
    }
  ]
}
```

Run `make build` in bscdot directory to build bscdot. You can then start a relayer as a binary using the default "Alice" key.

```bash
./build/bscdot --config config.json --testkey alice --latest  --verbosity trace
```

## Transfer in Polkadot.js.org