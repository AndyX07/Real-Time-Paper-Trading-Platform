#pragma once

#include <array>
#include <atomic>
#include <chrono>
#include <cstdint>
#include <memory>
#include <mutex>
#include <string>
#include <unordered_map>
#include <vector>

class LatencyHistogram {
public:
    explicit LatencyHistogram(std::string name) : name_{std::move(name)} {}

    struct Snapshot {
        std::string name;
        uint64_t count = 0;
        uint64_t p50 = 0;
        uint64_t p90 = 0;
        uint64_t p99 = 0;
        uint64_t max = 0;
    };

    void record(uint64_t nanos);
    Snapshot snapshot() const;
    void reset();

private:
    static constexpr size_t kNumBuckets = 64;

    std::string name_;
    std::array<std::atomic<uint64_t>, kNumBuckets> buckets_{};
    std::atomic<uint64_t> maxNanos_{0};
};

class HistogramRegistry {
public:
    static HistogramRegistry& instance();

    HistogramRegistry() = default;
    HistogramRegistry(const HistogramRegistry&) = delete;
    HistogramRegistry& operator=(const HistogramRegistry&) = delete;
    HistogramRegistry(HistogramRegistry&&) = delete;
    HistogramRegistry& operator=(HistogramRegistry&&) = delete;

    LatencyHistogram& get(const std::string& name);
    std::vector<LatencyHistogram::Snapshot> snapshotAll() const;

    void logAllAndReset(std::chrono::milliseconds flushInterval);

private:
    mutable std::mutex mutex_;
    std::unordered_map<std::string, std::unique_ptr<LatencyHistogram>> histograms_;
};

class ScopedLatencyTimer {
public:
    explicit ScopedLatencyTimer(LatencyHistogram& target)
        : target_{target}, start_{std::chrono::steady_clock::now()} {}

    ~ScopedLatencyTimer() {
        auto elapsed = std::chrono::steady_clock::now() - start_;
        target_.record(static_cast<uint64_t>(std::chrono::duration_cast<std::chrono::nanoseconds>(elapsed).count()));
    }

    ScopedLatencyTimer(const ScopedLatencyTimer&) = delete;
    ScopedLatencyTimer& operator=(const ScopedLatencyTimer&) = delete;

private:
    LatencyHistogram& target_;
    std::chrono::steady_clock::time_point start_;
};
