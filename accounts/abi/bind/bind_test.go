// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package bind

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var bindTests = []struct {
	name     string
	contract string
	bytecode []string
	abi      []string
	imports  string
	tester   string
	fsigs    []map[string]string
	libs     map[string]string
	aliases  map[string]string
	types    []string
}{
	// Test that the binding is available in combined and separate forms too
	{
		`Empty`,
		`contract NilContract {}`,
		[]string{`606060405260068060106000396000f3606060405200`},
		[]string{`[]`},
		`"github.com/ethereum/go-ethereum/common"`,
		`
			if b, err := NewEmpty(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("combined binding (%v) nil or error (%v) not nil", b, nil)
			}
			if b, err := NewEmptyCaller(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("caller binding (%v) nil or error (%v) not nil", b, nil)
			}
			if b, err := NewEmptyTransactor(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("transactor binding (%v) nil or error (%v) not nil", b, nil)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	// Test that all the official sample contracts bind correctly
	{
		`Token`,
		`https://ethereum.org/token`,
		[]string{`60606040526040516107fd3803806107fd83398101604052805160805160a05160c051929391820192909101600160a060020a0333166000908152600360209081526040822086905581548551838052601f6002600019610100600186161502019093169290920482018390047f290decd9548b62a8d60345a988386fc84ba6bc95484008f6362f93160ef3e56390810193919290918801908390106100e857805160ff19168380011785555b506101189291505b8082111561017157600081556001016100b4565b50506002805460ff19168317905550505050610658806101a56000396000f35b828001600101855582156100ac579182015b828111156100ac5782518260005055916020019190600101906100fa565b50508060016000509080519060200190828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061017557805160ff19168380011785555b506100c89291506100b4565b5090565b82800160010185558215610165579182015b8281111561016557825182600050559160200191906001019061018756606060405236156100775760e060020a600035046306fdde03811461007f57806323b872dd146100dc578063313ce5671461010e57806370a082311461011a57806395d89b4114610132578063a9059cbb1461018e578063cae9ca51146101bd578063dc3080f21461031c578063dd62ed3e14610341575b610365610002565b61036760008054602060026001831615610100026000190190921691909104601f810182900490910260809081016040526060828152929190828280156104eb5780601f106104c0576101008083540402835291602001916104eb565b6103d5600435602435604435600160a060020a038316600090815260036020526040812054829010156104f357610002565b6103e760025460ff1681565b6103d560043560036020526000908152604090205481565b610367600180546020600282841615610100026000190190921691909104601f810182900490910260809081016040526060828152929190828280156104eb5780601f106104c0576101008083540402835291602001916104eb565b610365600435602435600160a060020a033316600090815260036020526040902054819010156103f157610002565b60806020604435600481810135601f8101849004909302840160405260608381526103d5948235946024803595606494939101919081908382808284375094965050505050505060006000836004600050600033600160a060020a03168152602001908152602001600020600050600087600160a060020a031681526020019081526020016000206000508190555084905080600160a060020a0316638f4ffcb1338630876040518560e060020a0281526004018085600160a060020a0316815260200184815260200183600160a060020a03168152602001806020018281038252838181518152602001915080519060200190808383829060006004602084601f0104600f02600301f150905090810190601f1680156102f25780820380516001836020036101000a031916815260200191505b50955050505050506000604051808303816000876161da5a03f11561000257505050509392505050565b6005602090815260043560009081526040808220909252602435815220546103d59081565b60046020818152903560009081526040808220909252602435815220546103d59081565b005b60405180806020018281038252838181518152602001915080519060200190808383829060006004602084601f0104600f02600301f150905090810190601f1680156103c75780820380516001836020036101000a031916815260200191505b509250505060405180910390f35b60408051918252519081900360200190f35b6060908152602090f35b600160a060020a03821660009081526040902054808201101561041357610002565b806003600050600033600160a060020a03168152602001908152602001600020600082828250540392505081905550806003600050600084600160a060020a0316815260200190815260200160002060008282825054019250508190555081600160a060020a031633600160a060020a03167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a35050565b820191906000526020600020905b8154815290600101906020018083116104ce57829003601f168201915b505050505081565b600160a060020a03831681526040812054808301101561051257610002565b600160a060020a0380851680835260046020908152604080852033949094168086529382528085205492855260058252808520938552929052908220548301111561055c57610002565b816003600050600086600160a060020a03168152602001908152602001600020600082828250540392505081905550816003600050600085600160a060020a03168152602001908152602001600020600082828250540192505081905550816005600050600086600160a060020a03168152602001908152602001600020600050600033600160a060020a0316815260200190815260200160002060008282825054019250508190555082600160a060020a031633600160a060020a03167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef846040518082815260200191505060405180910390a3939250505056`},
		[]string{`[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"type":"function"},{"constant":false,"inputs":[{"name":"_from","type":"address"},{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"success","type":"bool"}],"type":"function"},{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"symbol","outputs":[{"name":"","type":"string"}],"type":"function"},{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[],"type":"function"},{"constant":false,"inputs":[{"name":"_spender","type":"address"},{"name":"_value","type":"uint256"},{"name":"_extraData","type":"bytes"}],"name":"approveAndCall","outputs":[{"name":"success","type":"bool"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"address"},{"name":"","type":"address"}],"name":"spentAllowance","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"address"},{"name":"","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"inputs":[{"name":"initialSupply","type":"uint256"},{"name":"tokenName","type":"string"},{"name":"decimalUnits","type":"uint8"},{"name":"tokenSymbol","type":"string"}],"type":"constructor"},{"anonymous":false,"inputs":[{"indexed":true,"name":"from","type":"address"},{"indexed":true,"name":"to","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Transfer","type":"event"}]`},
		`"github.com/ethereum/go-ethereum/common"`,
		`
			if b, err := NewToken(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("binding (%v) nil or error (%v) not nil", b, nil)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	{
		`Crowdsale`,
		`https://ethereum.org/crowdsale`,
		[]string{`606060408190526007805460ff1916905560a0806105a883396101006040529051608051915160c05160e05160008054600160a060020a03199081169095178155670de0b6b3a7640000958602600155603c9093024201600355930260045560058054909216909217905561052f90819061007990396000f36060604052361561006c5760e060020a600035046301cb3b20811461008257806329dcb0cf1461014457806338af3eed1461014d5780636e66f6e91461015f5780637a3a0e84146101715780637b3e5e7b1461017a578063a035b1fe14610183578063dc0d3dff1461018c575b61020060075460009060ff161561032357610002565b61020060035460009042106103205760025460015490106103cb576002548154600160a060020a0316908290606082818181858883f150915460025460408051600160a060020a039390931683526020830191909152818101869052517fe842aea7a5f1b01049d752008c53c52890b1a6daf660cf39e8eec506112bbdf6945090819003909201919050a15b60405160008054600160a060020a039081169230909116319082818181858883f150506007805460ff1916600117905550505050565b6103a160035481565b6103ab600054600160a060020a031681565b6103ab600554600160a060020a031681565b6103a160015481565b6103a160025481565b6103a160045481565b6103be60043560068054829081101561000257506000526002027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d3f8101547ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d409190910154600160a060020a03919091169082565b005b505050815481101561000257906000526020600020906002020160005060008201518160000160006101000a815481600160a060020a030219169083021790555060208201518160010160005055905050806002600082828250540192505081905550600560009054906101000a9004600160a060020a0316600160a060020a031663a9059cbb3360046000505484046040518360e060020a0281526004018083600160a060020a03168152602001828152602001925050506000604051808303816000876161da5a03f11561000257505060408051600160a060020a03331681526020810184905260018183015290517fe842aea7a5f1b01049d752008c53c52890b1a6daf660cf39e8eec506112bbdf692509081900360600190a15b50565b5060a0604052336060908152346080819052600680546001810180835592939282908280158290116102025760020281600202836000526020600020918201910161020291905b8082111561039d57805473ffffffffffffffffffffffffffffffffffffffff19168155600060019190910190815561036a565b5090565b6060908152602090f35b600160a060020a03166060908152602090f35b6060918252608052604090f35b5b60065481101561010e576006805482908110156100025760009182526002027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d3f0190600680549254600160a060020a0316928490811015610002576002027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d40015460405190915082818181858883f19350505050507fe842aea7a5f1b01049d752008c53c52890b1a6daf660cf39e8eec506112bbdf660066000508281548110156100025760008290526002027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d3f01548154600160a060020a039190911691908490811015610002576002027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d40015460408051600160a060020a0394909416845260208401919091526000838201525191829003606001919050a16001016103cc56`},
		[]string{`[{"constant":false,"inputs":[],"name":"checkGoalReached","outputs":[],"type":"function"},{"constant":true,"inputs":[],"name":"deadline","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"beneficiary","outputs":[{"name":"","type":"address"}],"type":"function"},{"constant":true,"inputs":[],"name":"tokenReward","outputs":[{"name":"","type":"address"}],"type":"function"},{"constant":true,"inputs":[],"name":"fundingGoal","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"amountRaised","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"price","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"funders","outputs":[{"name":"addr","type":"address"},{"name":"amount","type":"uint256"}],"type":"function"},{"inputs":[{"name":"ifSuccessfulSendTo","type":"address"},{"name":"fundingGoalInEthers","type":"uint256"},{"name":"durationInMinutes","type":"uint256"},{"name":"etherCostOfEachToken","type":"uint256"},{"name":"addressOfTokenUsedAsReward","type":"address"}],"type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"name":"backer","type":"address"},{"indexed":false,"name":"amount","type":"uint256"},{"indexed":false,"name":"isContribution","type":"bool"}],"name":"FundTransfer","type":"event"}]`},
		`"github.com/ethereum/go-ethereum/common"`,
		`
			if b, err := NewCrowdsale(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("binding (%v) nil or error (%v) not nil", b, nil)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	{
		`DAO`,
		`https://ethereum.org/dao`,
		[]string{`606060405260405160808061145f833960e06040529051905160a05160c05160008054600160a060020a03191633179055600184815560028490556003839055600780549182018082558280158290116100b8576003028160030283600052602060002091820191016100b891906101c8565b50506060919091015160029190910155600160a060020a0381166000146100a65760008054600160a060020a031916821790555b505050506111f18061026e6000396000f35b505060408051608081018252600080825260208281018290528351908101845281815292820192909252426060820152600780549194509250811015610002579081527fa66cc928b5edb82af9bd49922954155ab7b0942694bea4ce44661d9a8736c6889050815181546020848101517401000000000000000000000000000000000000000002600160a060020a03199290921690921760a060020a60ff021916178255604083015180516001848101805460008281528690209195600293821615610100026000190190911692909204601f9081018390048201949192919091019083901061023e57805160ff19168380011785555b50610072929150610226565b5050600060028201556001015b8082111561023a578054600160a860020a031916815560018181018054600080835592600290821615610100026000190190911604601f81901061020c57506101bb565b601f0160209004906000526020600020908101906101bb91905b8082111561023a5760008155600101610226565b5090565b828001600101855582156101af579182015b828111156101af57825182600050559160200191906001019061025056606060405236156100b95760e060020a6000350463013cf08b81146100bb578063237e9492146101285780633910682114610281578063400e3949146102995780635daf08ca146102a257806369bd34361461032f5780638160f0b5146103385780638da5cb5b146103415780639644fcbd14610353578063aa02a90f146103be578063b1050da5146103c7578063bcca1fd3146104b5578063d3c0715b146104dc578063eceb29451461058d578063f2fde38b1461067b575b005b61069c6004356004805482908110156100025790600052602060002090600a02016000506005810154815460018301546003840154600485015460068601546007870154600160a060020a03959095169750929560020194919360ff828116946101009093041692919089565b60408051602060248035600481810135601f81018590048502860185019096528585526107759581359591946044949293909201918190840183828082843750949650505050505050600060006004600050848154811015610002575090527f8a35acfbc15ff81a39ae7d344fd709f28e8600b4aa8c65c6b64bfe7fe36bd19e600a8402908101547f8a35acfbc15ff81a39ae7d344fd709f28e8600b4aa8c65c6b64bfe7fe36bd19b909101904210806101e65750600481015460ff165b8061026757508060000160009054906101000a9004600160a060020a03168160010160005054846040518084600160a060020a0316606060020a0281526014018381526020018280519060200190808383829060006004602084601f0104600f02600301f15090500193505050506040518091039020816007016000505414155b8061027757506001546005820154105b1561109257610002565b61077560043560066020526000908152604090205481565b61077560055481565b61078760043560078054829081101561000257506000526003026000805160206111d18339815191528101547fa66cc928b5edb82af9bd49922954155ab7b0942694bea4ce44661d9a8736c68a820154600160a060020a0382169260a060020a90920460ff16917fa66cc928b5edb82af9bd49922954155ab7b0942694bea4ce44661d9a8736c689019084565b61077560025481565b61077560015481565b610830600054600160a060020a031681565b604080516020604435600481810135601f81018490048402850184019095528484526100b9948135946024803595939460649492939101918190840183828082843750949650505050505050600080548190600160a060020a03908116339091161461084d57610002565b61077560035481565b604080516020604435600481810135601f8101849004840285018401909552848452610775948135946024803595939460649492939101918190840183828082843750506040805160209735808a0135601f81018a90048a0283018a019093528282529698976084979196506024909101945090925082915084018382808284375094965050505050505033600160a060020a031660009081526006602052604081205481908114806104ab5750604081205460078054909190811015610002579082526003026000805160206111d1833981519152015460a060020a900460ff16155b15610ce557610002565b6100b960043560243560443560005433600160a060020a03908116911614610b1857610002565b604080516020604435600481810135601f810184900484028501840190955284845261077594813594602480359593946064949293910191819084018382808284375094965050505050505033600160a060020a031660009081526006602052604081205481908114806105835750604081205460078054909190811015610002579082526003026000805160206111d18339815191520181505460a060020a900460ff16155b15610f1d57610002565b604080516020606435600481810135601f81018490048402850184019095528484526107759481359460248035956044359560849492019190819084018382808284375094965050505050505060006000600460005086815481101561000257908252600a027f8a35acfbc15ff81a39ae7d344fd709f28e8600b4aa8c65c6b64bfe7fe36bd19b01815090508484846040518084600160a060020a0316606060020a0281526014018381526020018280519060200190808383829060006004602084601f0104600f02600301f150905001935050505060405180910390208160070160005054149150610cdc565b6100b960043560005433600160a060020a03908116911614610f0857610002565b604051808a600160a060020a031681526020018981526020018060200188815260200187815260200186815260200185815260200184815260200183815260200182810382528981815460018160011615610100020316600290048152602001915080546001816001161561010002031660029004801561075e5780601f106107335761010080835404028352916020019161075e565b820191906000526020600020905b81548152906001019060200180831161074157829003601f168201915b50509a505050505050505050505060405180910390f35b60408051918252519081900360200190f35b60408051600160a060020a038616815260208101859052606081018390526080918101828152845460026001821615610100026000190190911604928201839052909160a08301908590801561081e5780601f106107f35761010080835404028352916020019161081e565b820191906000526020600020905b81548152906001019060200180831161080157829003601f168201915b50509550505050505060405180910390f35b60408051600160a060020a03929092168252519081900360200190f35b600160a060020a03851660009081526006602052604081205414156108a957604060002060078054918290556001820180825582801582901161095c5760030281600302836000526020600020918201910161095c9190610a4f565b600160a060020a03851660009081526006602052604090205460078054919350908390811015610002575060005250600381026000805160206111d183398151915201805474ff0000000000000000000000000000000000000000191660a060020a85021781555b60408051600160a060020a03871681526020810186905281517f27b022af4a8347100c7a041ce5ccf8e14d644ff05de696315196faae8cd50c9b929181900390910190a15050505050565b505050915081506080604051908101604052808681526020018581526020018481526020014281526020015060076000508381548110156100025790600052602060002090600302016000508151815460208481015160a060020a02600160a060020a03199290921690921774ff00000000000000000000000000000000000000001916178255604083015180516001848101805460008281528690209195600293821615610100026000190190911692909204601f90810183900482019491929190910190839010610ad357805160ff19168380011785555b50610b03929150610abb565b5050600060028201556001015b80821115610acf57805474ffffffffffffffffffffffffffffffffffffffffff1916815560018181018054600080835592600290821615610100026000190190911604601f819010610aa15750610a42565b601f016020900490600052602060002090810190610a4291905b80821115610acf5760008155600101610abb565b5090565b82800160010185558215610a36579182015b82811115610a36578251826000505591602001919060010190610ae5565b50506060919091015160029190910155610911565b600183905560028290556003819055604080518481526020810184905280820183905290517fa439d3fa452be5e0e1e24a8145e715f4fd8b9c08c96a42fd82a855a85e5d57de9181900360600190a1505050565b50508585846040518084600160a060020a0316606060020a0281526014018381526020018280519060200190808383829060006004602084601f0104600f02600301f150905001935050505060405180910390208160070160005081905550600260005054603c024201816003016000508190555060008160040160006101000a81548160ff0219169083021790555060008160040160016101000a81548160ff02191690830217905550600081600501600050819055507f646fec02522b41e7125cfc859a64fd4f4cefd5dc3b6237ca0abe251ded1fa881828787876040518085815260200184600160a060020a03168152602001838152602001806020018281038252838181518152602001915080519060200190808383829060006004602084601f0104600f02600301f150905090810190601f168015610cc45780820380516001836020036101000a031916815260200191505b509550505050505060405180910390a1600182016005555b50949350505050565b6004805460018101808355909190828015829011610d1c57600a0281600a028360005260206000209182019101610d1c9190610db8565b505060048054929450918491508110156100025790600052602060002090600a02016000508054600160a060020a031916871781556001818101879055855160028381018054600082815260209081902096975091959481161561010002600019011691909104601f90810182900484019391890190839010610ed857805160ff19168380011785555b50610b6c929150610abb565b50506001015b80821115610acf578054600160a060020a03191681556000600182810182905560028381018054848255909281161561010002600019011604601f819010610e9c57505b5060006003830181905560048301805461ffff191690556005830181905560068301819055600783018190556008830180548282559082526020909120610db2916002028101905b80821115610acf57805474ffffffffffffffffffffffffffffffffffffffffff1916815560018181018054600080835592600290821615610100026000190190911604601f819010610eba57505b5050600101610e44565b601f016020900490600052602060002090810190610dfc9190610abb565b601f016020900490600052602060002090810190610e929190610abb565b82800160010185558215610da6579182015b82811115610da6578251826000505591602001919060010190610eea565b60008054600160a060020a0319168217905550565b600480548690811015610002576000918252600a027f8a35acfbc15ff81a39ae7d344fd709f28e8600b4aa8c65c6b64bfe7fe36bd19b01905033600160a060020a0316600090815260098201602052604090205490915060ff1660011415610f8457610002565b33600160a060020a031660009081526009820160205260409020805460ff1916600190811790915560058201805490910190558315610fcd576006810180546001019055610fda565b6006810180546000190190555b7fc34f869b7ff431b034b7b9aea9822dac189a685e0b015c7d1be3add3f89128e8858533866040518085815260200184815260200183600160a060020a03168152602001806020018281038252838181518152602001915080519060200190808383829060006004602084601f0104600f02600301f150905090810190601f16801561107a5780820380516001836020036101000a031916815260200191505b509550505050505060405180910390a1509392505050565b6006810154600354901315611158578060000160009054906101000a9004600160a060020a0316600160a060020a03168160010160005054670de0b6b3a76400000284604051808280519060200190808383829060006004602084601f0104600f02600301f150905090810190601f1680156111225780820380516001836020036101000a031916815260200191505b5091505060006040518083038185876185025a03f15050505060048101805460ff191660011761ff00191661010017905561116d565b60048101805460ff191660011761ff00191690555b60068101546005820154600483015460408051888152602081019490945283810192909252610100900460ff166060830152517fd220b7272a8b6d0d7d6bcdace67b936a8f175e6d5c1b3ee438b72256b32ab3af9181900360800190a1509291505056a66cc928b5edb82af9bd49922954155ab7b0942694bea4ce44661d9a8736c688`},
		[]string{`[{"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"proposals","outputs":[{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"},{"name":"description","type":"string"},{"name":"votingDeadline","type":"uint256"},{"name":"executed","type":"bool"},{"name":"proposalPassed","type":"bool"},{"name":"numberOfVotes","type":"uint256"},{"name":"currentResult","type":"int256"},{"name":"proposalHash","type":"bytes32"}],"type":"function"},{"constant":false,"inputs":[{"name":"proposalNumber","type":"uint256"},{"name":"transactionBytecode","type":"bytes"}],"name":"executeProposal","outputs":[{"name":"result","type":"int256"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"address"}],"name":"memberId","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"numProposals","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[{"name":"","type":"uint256"}],"name":"members","outputs":[{"name":"member","type":"address"},{"name":"canVote","type":"bool"},{"name":"name","type":"string"},{"name":"memberSince","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"debatingPeriodInMinutes","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"minimumQuorum","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[],"name":"owner","outputs":[{"name":"","type":"address"}],"type":"function"},{"constant":false,"inputs":[{"name":"targetMember","type":"address"},{"name":"canVote","type":"bool"},{"name":"memberName","type":"string"}],"name":"changeMembership","outputs":[],"type":"function"},{"constant":true,"inputs":[],"name":"majorityMargin","outputs":[{"name":"","type":"int256"}],"type":"function"},{"constant":false,"inputs":[{"name":"beneficiary","type":"address"},{"name":"etherAmount","type":"uint256"},{"name":"JobDescription","type":"string"},{"name":"transactionBytecode","type":"bytes"}],"name":"newProposal","outputs":[{"name":"proposalID","type":"uint256"}],"type":"function"},{"constant":false,"inputs":[{"name":"minimumQuorumForProposals","type":"uint256"},{"name":"minutesForDebate","type":"uint256"},{"name":"marginOfVotesForMajority","type":"int256"}],"name":"changeVotingRules","outputs":[],"type":"function"},{"constant":false,"inputs":[{"name":"proposalNumber","type":"uint256"},{"name":"supportsProposal","type":"bool"},{"name":"justificationText","type":"string"}],"name":"vote","outputs":[{"name":"voteID","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[{"name":"proposalNumber","type":"uint256"},{"name":"beneficiary","type":"address"},{"name":"etherAmount","type":"uint256"},{"name":"transactionBytecode","type":"bytes"}],"name":"checkProposalCode","outputs":[{"name":"codeChecksOut","type":"bool"}],"type":"function"},{"constant":false,"inputs":[{"name":"newOwner","type":"address"}],"name":"transferOwnership","outputs":[],"type":"function"},{"inputs":[{"name":"minimumQuorumForProposals","type":"uint256"},{"name":"minutesForDebate","type":"uint256"},{"name":"marginOfVotesForMajority","type":"int256"},{"name":"congressLeader","type":"address"}],"type":"constructor"},{"anonymous":false,"inputs":[{"indexed":false,"name":"proposalID","type":"uint256"},{"indexed":false,"name":"recipient","type":"address"},{"indexed":false,"name":"amount","type":"uint256"},{"indexed":false,"name":"description","type":"string"}],"name":"ProposalAdded","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"proposalID","type":"uint256"},{"indexed":false,"name":"position","type":"bool"},{"indexed":false,"name":"voter","type":"address"},{"indexed":false,"name":"justification","type":"string"}],"name":"Voted","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"proposalID","type":"uint256"},{"indexed":false,"name":"result","type":"int256"},{"indexed":false,"name":"quorum","type":"uint256"},{"indexed":false,"name":"active","type":"bool"}],"name":"ProposalTallied","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"member","type":"address"},{"indexed":false,"name":"isMember","type":"bool"}],"name":"MembershipChanged","type":"event"},{"anonymous":false,"inputs":[{"indexed":false,"name":"minimumQuorum","type":"uint256"},{"indexed":false,"name":"debatingPeriodInMinutes","type":"uint256"},{"indexed":false,"name":"majorityMargin","type":"int256"}],"name":"ChangeOfRules","type":"event"}]`},
		`"github.com/ethereum/go-ethereum/common"`,
		`
			if b, err := NewDAO(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("binding (%v) nil or error (%v) not nil", b, nil)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	// Test that named and anonymous inputs are handled correctly
	{
		`InputChecker`, ``, []string{``},
		[]string{`
			[
				{"type":"function","name":"noInput","constant":true,"inputs":[],"outputs":[]},
				{"type":"function","name":"namedInput","constant":true,"inputs":[{"name":"str","type":"string"}],"outputs":[]},
				{"type":"function","name":"anonInput","constant":true,"inputs":[{"name":"","type":"string"}],"outputs":[]},
				{"type":"function","name":"namedInputs","constant":true,"inputs":[{"name":"str1","type":"string"},{"name":"str2","type":"string"}],"outputs":[]},
				{"type":"function","name":"anonInputs","constant":true,"inputs":[{"name":"","type":"string"},{"name":"","type":"string"}],"outputs":[]},
				{"type":"function","name":"mixedInputs","constant":true,"inputs":[{"name":"","type":"string"},{"name":"str","type":"string"}],"outputs":[]}
			]
		`},
		`
			"fmt"

			"github.com/ethereum/go-ethereum/common"
		`,
		`if b, err := NewInputChecker(common.Address{}, nil); b == nil || err != nil {
			 t.Fatalf("binding (%v) nil or error (%v) not nil", b, nil)
		 } else if false { // Don't run, just compile and test types
			 var err error

			 err = b.NoInput(nil)
			 err = b.NamedInput(nil, "")
			 err = b.AnonInput(nil, "")
			 err = b.NamedInputs(nil, "", "")
			 err = b.AnonInputs(nil, "", "")
			 err = b.MixedInputs(nil, "", "")

			 fmt.Println(err)
		 }`,
		nil,
		nil,
		nil,
		nil,
	},
	// Test that named and anonymous outputs are handled correctly
	{
		`OutputChecker`, ``, []string{``},
		[]string{`
			[
				{"type":"function","name":"noOutput","constant":true,"inputs":[],"outputs":[]},
				{"type":"function","name":"namedOutput","constant":true,"inputs":[],"outputs":[{"name":"str","type":"string"}]},
				{"type":"function","name":"anonOutput","constant":true,"inputs":[],"outputs":[{"name":"","type":"string"}]},
				{"type":"function","name":"namedOutputs","constant":true,"inputs":[],"outputs":[{"name":"str1","type":"string"},{"name":"str2","type":"string"}]},
				{"type":"function","name":"collidingOutputs","constant":true,"inputs":[],"outputs":[{"name":"str","type":"string"},{"name":"Str","type":"string"}]},
				{"type":"function","name":"anonOutputs","constant":true,"inputs":[],"outputs":[{"name":"","type":"string"},{"name":"","type":"string"}]},
				{"type":"function","name":"mixedOutputs","constant":true,"inputs":[],"outputs":[{"name":"","type":"string"},{"name":"str","type":"string"}]}
			]
		`},
		`
			"fmt"

			"github.com/ethereum/go-ethereum/common"
		`,
		`if b, err := NewOutputChecker(common.Address{}, nil); b == nil || err != nil {
			 t.Fatalf("binding (%v) nil or error (%v) not nil", b, nil)
		 } else if false { // Don't run, just compile and test types
			 var str1, str2 string
			 var err error

			 err              = b.NoOutput(nil)
			 str1, err        = b.NamedOutput(nil)
			 str1, err        = b.AnonOutput(nil)
			 res, _          := b.NamedOutputs(nil)
			 str1, str2, err  = b.CollidingOutputs(nil)
			 str1, str2, err  = b.AnonOutputs(nil)
			 str1, str2, err  = b.MixedOutputs(nil)

			 fmt.Println(str1, str2, res.Str1, res.Str2, err)
		 }`,
		nil,
		nil,
		nil,
		nil,
	},
	// Tests that named, anonymous and indexed events are handled correctly
	{
		`EventChecker`, ``, []string{``},
		[]string{`
			[
				{"type":"event","name":"empty","inputs":[]},
				{"type":"event","name":"indexed","inputs":[{"name":"addr","type":"address","indexed":true},{"name":"num","type":"int256","indexed":true}]},
				{"type":"event","name":"mixed","inputs":[{"name":"addr","type":"address","indexed":true},{"name":"num","type":"int256"}]},
				{"type":"event","name":"anonymous","anonymous":true,"inputs":[]},
				{"type":"event","name":"dynamic","inputs":[{"name":"idxStr","type":"string","indexed":true},{"name":"idxDat","type":"bytes","indexed":true},{"name":"str","type":"string"},{"name":"dat","type":"bytes"}]},
				{"type":"event","name":"unnamed","inputs":[{"name":"","type":"uint256","indexed": true},{"name":"","type":"uint256","indexed":true}]}
			]
		`},
		`
			"fmt"
			"math/big"
			"reflect"

			"github.com/ethereum/go-ethereum/common"
		`,
		`if e, err := NewEventChecker(common.Address{}, nil); e == nil || err != nil {
			 t.Fatalf("binding (%v) nil or error (%v) not nil", e, nil)
		 } else if false { // Don't run, just compile and test types
			 var (
				 err  error
			   res  bool
				 str  string
				 dat  []byte
				 hash common.Hash
			 )
			 _, err = e.FilterEmpty(nil)
			 _, err = e.FilterIndexed(nil, []common.Address{}, []*big.Int{})

			 mit, err := e.FilterMixed(nil, []common.Address{})

			 res = mit.Next()  // Make sure the iterator has a Next method
			 err = mit.Error() // Make sure the iterator has an Error method
			 err = mit.Close() // Make sure the iterator has a Close method

			 fmt.Println(mit.Event.Raw.BlockHash) // Make sure the raw log is contained within the results
			 fmt.Println(mit.Event.Num)           // Make sure the unpacked non-indexed fields are present
			 fmt.Println(mit.Event.Addr)          // Make sure the reconstructed indexed fields are present

			 dit, err := e.FilterDynamic(nil, []string{}, [][]byte{})

			 str  = dit.Event.Str    // Make sure non-indexed strings retain their type
			 dat  = dit.Event.Dat    // Make sure non-indexed bytes retain their type
			 hash = dit.Event.IdxStr // Make sure indexed strings turn into hashes
			 hash = dit.Event.IdxDat // Make sure indexed bytes turn into hashes

			 sink := make(chan *EventCheckerMixed)
			 sub, err := e.WatchMixed(nil, sink, []common.Address{})
			 defer sub.Unsubscribe()

			 event := <-sink
			 fmt.Println(event.Raw.BlockHash) // Make sure the raw log is contained within the results
			 fmt.Println(event.Num)           // Make sure the unpacked non-indexed fields are present
			 fmt.Println(event.Addr)          // Make sure the reconstructed indexed fields are present

			 fmt.Println(res, str, dat, hash, err)

			 oit, err := e.FilterUnnamed(nil, []*big.Int{}, []*big.Int{})

			 arg0  := oit.Event.Arg0    // Make sure unnamed arguments are handled correctly
			 arg1  := oit.Event.Arg1    // Make sure unnamed arguments are handled correctly
			 fmt.Println(arg0, arg1)
		 }
		 // Run a tiny reflection test to ensure disallowed methods don't appear
		 if _, ok := reflect.TypeOf(&EventChecker{}).MethodByName("FilterAnonymous"); ok {
		 	t.Errorf("binding has disallowed method (FilterAnonymous)")
		 }`,
		nil,
		nil,
		nil,
		nil,
	},
	// Tests that non-existent contracts are reported as such (though only simulator test)
	{
		`NonExistent`,
		`
			contract NonExistent {
				function String() constant returns(string) {
					return "I don't exist";
				}
			}
		`,
		[]string{`6060604052609f8060106000396000f3606060405260e060020a6000350463f97a60058114601a575b005b600060605260c0604052600d60809081527f4920646f6e27742065786973740000000000000000000000000000000000000060a052602060c0908152600d60e081905281906101009060a09080838184600060046012f15050815172ffffffffffffffffffffffffffffffffffffff1916909152505060405161012081900392509050f3`},
		[]string{`[{"constant":true,"inputs":[],"name":"String","outputs":[{"name":"","type":"string"}],"type":"function"}]`},
		`
			"github.com/ethereum/go-ethereum/accounts/abi/bind"
			"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
			"github.com/ethereum/go-ethereum/common"
			"github.com/ethereum/go-ethereum/core/types"
		`,
		`
			// Create a simulator and wrap a non-deployed contract

			sim := backends.NewSimulatedBackend(types.GenesisAlloc{}, uint64(10000000000))
			defer sim.Close()

			nonexistent, err := NewNonExistent(common.Address{}, sim)
			if err != nil {
				t.Fatalf("Failed to access non-existent contract: %v", err)
			}
			// Ensure that contract calls fail with the appropriate error
			if res, err := nonexistent.String(nil); err == nil {
				t.Fatalf("Call succeeded on non-existent contract: %v", res)
			} else if (err != bind.ErrNoCode) {
				t.Fatalf("Error mismatch: have %v, want %v", err, bind.ErrNoCode)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	{
		`NonExistentStruct`,
		`
			contract NonExistentStruct {
				function Struct() public view returns(uint256 a, uint256 b) {
					return (10, 10);
				}
			}
		`,
		[]string{`6080604052348015600f57600080fd5b5060888061001e6000396000f3fe6080604052348015600f57600080fd5b506004361060285760003560e01c8063d5f6622514602d575b600080fd5b6033604c565b6040805192835260208301919091528051918290030190f35b600a809156fea264697066735822beefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeefbeef64736f6c6343decafe0033`},
		[]string{`[{"inputs":[],"name":"Struct","outputs":[{"internalType":"uint256","name":"a","type":"uint256"},{"internalType":"uint256","name":"b","type":"uint256"}],"stateMutability":"pure","type":"function"}]`},
		`
			"github.com/ethereum/go-ethereum/accounts/abi/bind"
			"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
			"github.com/ethereum/go-ethereum/common"
			"github.com/ethereum/go-ethereum/core/types"
		`,
		`
			// Create a simulator and wrap a non-deployed contract

			sim := backends.NewSimulatedBackend(types.GenesisAlloc{}, uint64(10000000000))
			defer sim.Close()

			nonexistent, err := NewNonExistentStruct(common.Address{}, sim)
			if err != nil {
				t.Fatalf("Failed to access non-existent contract: %v", err)
			}
			// Ensure that contract calls fail with the appropriate error
			if res, err := nonexistent.Struct(nil); err == nil {
				t.Fatalf("Call succeeded on non-existent contract: %v", res)
			} else if (err != bind.ErrNoCode) {
				t.Fatalf("Error mismatch: have %v, want %v", err, bind.ErrNoCode)
			}
		`,
		nil,
		nil,
		nil,
		nil,
	},
	{
		"IdentifierCollision",
		`
		pragma solidity >=0.4.19 <0.6.0;

		contract IdentifierCollision {
			uint public _myVar;

			function MyVar() public view returns (uint) {
				return _myVar;
			}
		}
		`,
		[]string{"60806040523480156100115760006000fd5b50610017565b60c3806100256000396000f3fe608060405234801560105760006000fd5b506004361060365760003560e01c806301ad4d8714603c5780634ef1f0ad146058576036565b60006000fd5b60426074565b6040518082815260200191505060405180910390f35b605e607d565b6040518082815260200191505060405180910390f35b60006000505481565b60006000600050549050608b565b9056fea265627a7a7231582067c8d84688b01c4754ba40a2a871cede94ea1f28b5981593ab2a45b46ac43af664736f6c634300050c0032"},
		[]string{`[{"constant":true,"inputs":[],"name":"MyVar","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[],"name":"_myVar","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"}]`},
		`
		"math/big"

		"github.com/ethereum/go-ethereum/accounts/abi/bind"
		"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
		"github.com/ethereum/go-ethereum/crypto"
		"github.com/ethereum/go-ethereum/core/types"
		`,
		`
		// Initialize test accounts
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)

		// Deploy registrar contract
		sim := backends.NewSimulatedBackend(types.GenesisAlloc{addr: {Balance: big.NewInt(10000000000000000)}}, 10000000)
		defer sim.Close()

		transactOpts, _ := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337))
		_, _, _, err := DeployIdentifierCollision(transactOpts, sim)
		if err != nil {
			t.Fatalf("failed to deploy contract: %v", err)
		}
		`,
		nil,
		nil,
		map[string]string{"_myVar": "pubVar"}, // alias MyVar to PubVar
		nil,
	},
	{
		name: "NumericMethodName",
		contract: `
		// SPDX-License-Identifier: GPL-3.0
		pragma solidity >=0.4.22 <0.9.0;

		contract NumericMethodName {
			event _1TestEvent(address _param);
			function _1test() public pure {}
			function __1test() public pure {}
			function __2test() public pure {}
		}
		`,
		bytecode: []string{"0x6080604052348015600f57600080fd5b5060958061001e6000396000f3fe6080604052348015600f57600080fd5b5060043610603c5760003560e01c80639d993132146041578063d02767c7146049578063ffa02795146051575b600080fd5b60476059565b005b604f605b565b005b6057605d565b005b565b565b56fea26469706673582212200382ca602dff96a7e2ba54657985e2b4ac423a56abe4a1f0667bc635c4d4371f64736f6c63430008110033"},
		abi:      []string{`[{"anonymous":false,"inputs":[{"indexed":false,"internalType":"address","name":"_param","type":"address"}],"name":"_1TestEvent","type":"event"},{"inputs":[],"name":"_1test","outputs":[],"stateMutability":"pure","type":"function"},{"inputs":[],"name":"__1test","outputs":[],"stateMutability":"pure","type":"function"},{"inputs":[],"name":"__2test","outputs":[],"stateMutability":"pure","type":"function"}]`},
		imports: `
			"github.com/ethereum/go-ethereum/common"
		`,
		tester: `
			if b, err := NewNumericMethodName(common.Address{}, nil); b == nil || err != nil {
				t.Fatalf("combined binding (%v) nil or error (%v) not nil", b, nil)
			}
`,
	},
}

// Tests that packages generated by the binder can be successfully compiled and
// the requested tester run against it.
func TestGolangBindings(t *testing.T) {
	t.Parallel()
	// Skip the test if no Go command can be found
	gocmd := runtime.GOROOT() + "/bin/go"
	if !common.FileExist(gocmd) {
		t.Skip("go sdk not found for testing")
	}
	// Create a temporary workspace for the test suite
	ws := t.TempDir()

	pkg := filepath.Join(ws, "bindtest")
	if err := os.MkdirAll(pkg, 0700); err != nil {
		t.Fatalf("failed to create package: %v", err)
	}
	// Generate the test suite for all the contracts
	for i, tt := range bindTests {
		t.Run(tt.name, func(t *testing.T) {
			var types []string
			if tt.types != nil {
				types = tt.types
			} else {
				types = []string{tt.name}
			}
			// Generate the binding and create a Go source file in the workspace
			bind, err := Bind(types, tt.abi, tt.bytecode, tt.fsigs, "bindtest", LangGo, tt.libs, tt.aliases)
			if err != nil {
				t.Fatalf("test %d: failed to generate binding: %v", i, err)
			}
			if err = os.WriteFile(filepath.Join(pkg, strings.ToLower(tt.name)+".go"), []byte(bind), 0600); err != nil {
				t.Fatalf("test %d: failed to write binding: %v", i, err)
			}
			// Generate the test file with the injected test code
			code := fmt.Sprintf(`
			package bindtest

			import (
				"testing"
				%s
			)

			func Test%s(t *testing.T) {
				%s
			}
		`, tt.imports, tt.name, tt.tester)
			if err := os.WriteFile(filepath.Join(pkg, strings.ToLower(tt.name)+"_test.go"), []byte(code), 0600); err != nil {
				t.Fatalf("test %d: failed to write tests: %v", i, err)
			}
		})
	}
	// Convert the package to go modules and use the current source for go-ethereum
	moder := exec.Command(gocmd, "mod", "init", "bindtest")
	moder.Dir = pkg
	if out, err := moder.CombinedOutput(); err != nil {
		t.Fatalf("failed to convert binding test to modules: %v\n%s", err, out)
	}
	pwd, _ := os.Getwd()
	replacer := exec.Command(gocmd, "mod", "edit", "-x", "-require", "github.com/ethereum/go-ethereum@v0.0.0", "-replace", "github.com/ethereum/go-ethereum="+filepath.Join(pwd, "..", "..", "..")) // Repo root
	replacer.Dir = pkg
	if out, err := replacer.CombinedOutput(); err != nil {
		t.Fatalf("failed to replace binding test dependency to current source tree: %v\n%s", err, out)
	}
	tidier := exec.Command(gocmd, "mod", "tidy")
	tidier.Dir = pkg
	if out, err := tidier.CombinedOutput(); err != nil {
		t.Fatalf("failed to tidy Go module file: %v\n%s", err, out)
	}
	// Test the entire package and report any failures
	cmd := exec.Command(gocmd, "test", "-v", "-count", "1")
	cmd.Dir = pkg
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to run binding test: %v\n%s", err, out)
	}
}
