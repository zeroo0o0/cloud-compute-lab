package main

import "testing"

func BenchmarkCPUHotspotBad(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = buildRankDigestSlow(4000)
	}
}

func BenchmarkCPUHotspotGood(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = buildRankDigestFast(4000)
	}
}

func BenchmarkHeapAllocBad(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = encodeBattleLogBad(5000)
	}
}

func BenchmarkHeapAllocGood(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = encodeBattleLogGood(5000)
	}
}

func BenchmarkMutexContentionBad(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mergeRoomDamageBad(16, 20000)
	}
}

func BenchmarkMutexContentionGood(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mergeRoomDamageGood(16, 20000)
	}
}
