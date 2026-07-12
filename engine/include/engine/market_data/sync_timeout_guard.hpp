#pragma once

#include <chrono>
#include <condition_variable>
#include <mutex>
#include <thread>

#include <boost/beast/core/tcp_stream.hpp>

class SyncTimeoutGuard {
public:
    SyncTimeoutGuard(boost::beast::tcp_stream& stream, std::chrono::steady_clock::duration timeout)
        : thread_([this, &stream, timeout] {
              std::unique_lock<std::mutex> lock(mutex_);
              if (!cv_.wait_for(lock, timeout, [this] { return done_; })) {
                  boost::system::error_code ec;
                  stream.socket().close(ec);
              }
          }) {}

    SyncTimeoutGuard(const SyncTimeoutGuard&) = delete;
    SyncTimeoutGuard& operator=(const SyncTimeoutGuard&) = delete;

    ~SyncTimeoutGuard() {
        {
            std::lock_guard<std::mutex> lock(mutex_);
            done_ = true;
        }
        cv_.notify_one();
        thread_.join();
    }

private:
    std::mutex mutex_;
    std::condition_variable cv_;
    bool done_ = false;
    std::thread thread_;
};
