package core

import (
	"fmt"
	"math/big"
)

var (
	// Rewards plan of rule 1
	rewardsByMonth1 = []*big.Int{
		fromGwei(460590277777778), fromGwei(516685474537037), fromGwei(573248131269290), fromGwei(630282143474312), fromGwei(687791439114376), fromGwei(745779978884773),
		fromGwei(838973978708813), fromGwei(932944595198053), fromGwei(1027698300158040), fromGwei(1123241619326020), fromGwei(1219581132820400), fromGwei(1316723475593910),
		fromGwei(1449397560112750), fromGwei(1583177262002570), fromGwei(1718071794741480), fromGwei(1854090448586550), fromGwei(1991242591213660), fromGwei(2129537668362660),
		fromGwei(2268985204487910), fromGwei(2409594803414200), fromGwei(2551376148998200), fromGwei(2694339005795410), fromGwei(2838493219732590), fromGwei(2983848718785920),
		fromGwei(3154721069220250), fromGwei(3327017355908200), fromGwei(3500749444985210), fromGwei(3675929301471200), fromGwei(3852568990094570), fromGwei(4030680676123140),
		fromGwei(4210276626201940), fromGwei(4391369209198070), fromGwei(4573970897052500), fromGwei(4758094265639040), fromGwei(4943751995630480), fromGwei(5130956873371840),
		fromGwei(5295416236205500), fromGwei(5461246093729430), fromGwei(5628457866732730), fromGwei(5797063071177730), fromGwei(5967073318993100), fromGwei(6138500318873600),
		fromGwei(6276633654864210), fromGwei(6415918101988080), fromGwei(6556363252837980), fromGwei(6697978779944960), fromGwei(6840774436444510), fromGwei(6984760056748220),
	}

	// Default rewards plan, which is rule 2
	rewardsByMonth2 = []*big.Int{
		fromGwei(384027777800000), fromGwei(446255787000000), fromGwei(509002363000000), fromGwei(572271827200000), fromGwei(636068536800000), fromGwei(700396885800000),
		fromGwei(799983526500000), fromGwei(900400055900000), fromGwei(1001653390000000), fromGwei(1095417168000000), fromGwei(1189962311000000), fromGwei(1285295330000000),
		fromGwei(1416145014000000), fromGwei(1548085111000000), fromGwei(1681124709000000), fromGwei(1815272970000000), fromGwei(1950539134000000), fromGwei(2086932516000000),
		fromGwei(2224462509000000), fromGwei(2363138585000000), fromGwei(2502970296000000), fromGwei(2643967271000000), fromGwei(2786139220000000), fromGwei(2929495936000000),
		fromGwei(3098352846000000), fromGwei(3268616898000000), fromGwei(3440299816000000), fromGwei(3613413426000000), fromGwei(3787969649000000), fromGwei(3963980507000000),
		fromGwei(4141458123000000), fromGwei(4320414718000000), fromGwei(4500862618000000), fromGwei(4682814251000000), fromGwei(4866282148000000), fromGwei(5051278944000000),
		fromGwei(5213511824000000), fromGwei(5377096644000000), fromGwei(5542044672000000), fromGwei(5708367267000000), fromGwei(5876075883000000), fromGwei(6045182071000000),
		fromGwei(6180975254000000), fromGwei(6317900048000000), fromGwei(6455965882000000), fromGwei(6595182264000000), fromGwei(6735558783000000), fromGwei(6877105106000000),
	}

	rewardsPlans = [][]*big.Int{
		rewardsByMonth1,
		rewardsByMonth2,
	}
)

// fromGwei convert amount from gwei to wei
func fromGwei(gwei int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(gwei), big.NewInt(1000000000))
}

// RewardsByMonth returns RewardsByMonth info by different rules
// rule begins from 1, and 0 will return default value
func RewardsByMonth(rule uint64) []*big.Int {
	var idx int
	if rule == 0 { // 0 is the latest rule by default
		idx = len(rewardsPlans) - 1
	} else {
		idx = int(rule) - 1
	}
	if idx >= len(rewardsPlans) {
		panic(fmt.Sprintf("Unknown turbo rewards rule: %v\n", rule))
	}
	return rewardsPlans[idx]
}
