#include "engine/observability/histogram.hpp"

#include <algorithm>
#include <bit>
#include <iostream>
#include <thread>

namespace {

size_t bucketFor(uint64_t nanos) {
    if (nanos == 0) {
        return 0;
    }
    int bucket = 63 - std::countl_zero(nanos);
    return static_cast<size_t>(std::clamp(bucket, 0, 63));
}

}

void LatencyHistogram::record(uint64_t nanos) {
    buckets_[bucketFor(nanos)].fetch_add(1, std::memory_order_relaxed);

    uint64_t prevMax = maxNanos_.load(std::memory_order_relaxed);
    while (nanos > prevMax && !maxNanos_.compare_exchange_weak(prevMax, nanos, std::memory_order_relaxed)) {
    }
}

LatencyHistogram::Snapshot LatencyHistogram::snapshot() const {
    std::array<uint64_t, kNumBuckets> counts{};
    uint64_t total = 0;
    for (size_t i = 0; i < kNumBuckets; ++i) {
        counts[i] = buckets_[i].load(std::memory_order_relaxed);
        total += counts[i];
    }

    Snapshot s;
    s.name = name_;
    s.count = total;
    s.max = maxNanos_.load(std::memory_order_relaxed);
    if (total == 0) {
        return s;
    }

    auto percentileValue = [&](double fraction) -> uint64_t {
        uint64_t target = std::max<uint64_t>(1, static_cast<uint64_t>(fraction * static_cast<double>(total)));
        uint64_t cumulative = 0;
        for (size_t i = 0; i < kNumBuckets; ++i) {
            cumulative += counts[i];
            if (cumulative >= target) {
                return uint64_t{1} << i;
            }
        }
        return uint64_t{1} << (kNumBuckets - 1);
    };

    s.p50 = percentileValue(0.50);
    s.p90 = percentileValue(0.90);
    s.p99 = percentileValue(0.99);
    return s;
}

void LatencyHistogram::reset() {
    for (auto& bucket : buckets_) {
        bucket.store(0, std::memory_order_relaxed);
    }
    maxNanos_.store(0, std::memory_order_relaxed);
}

HistogramRegistry& HistogramRegistry::instance() {
    static HistogramRegistry registry;
    return registry;
}

LatencyHistogram& HistogramRegistry::get(const std::string& name) {
    std::lock_guard<std::mutex> lock{mutex_};
    auto it = histograms_.find(name);
    if (it == histograms_.end()) {
        it = histograms_.emplace(name, std::make_unique<LatencyHistogram>(name)).first;
    }
    return *it->second;
}

std::vector<LatencyHistogram::Snapshot> HistogramRegistry::snapshotAll() const {
    std::lock_guard<std::mutex> lock{mutex_};
    std::vector<LatencyHistogram::Snapshot> snapshots;
    snapshots.reserve(histograms_.size());
    for (const auto& [name, hist] : histograms_) {
        snapshots.push_back(hist->snapshot());
    }
    return snapshots;
}

void HistogramRegistry::logAllAndReset(std::chrono::milliseconds flushInterval) {
    while (true) {
        std::this_thread::sleep_for(flushInterval);

        for (const auto& s : snapshotAll()) {
            if (s.count == 0) {
                continue;
            }
            std::cout << "engine: perf." << s.name << " count=" << s.count << " p50=" << s.p50 << "ns p90=" << s.p90
                       << "ns p99=" << s.p99 << "ns max=" << s.max << "ns\n";
        }

        std::lock_guard<std::mutex> lock{mutex_};
        for (auto& [name, hist] : histograms_) {
            hist->reset();
        }
    }
}
