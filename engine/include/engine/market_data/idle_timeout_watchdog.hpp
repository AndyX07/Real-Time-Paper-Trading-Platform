#pragma once

#include <chrono>
#include <condition_variable>
#include <mutex>
#include <thread>

#include <boost/beast/core/tcp_stream.hpp>

// Closes the socket if kick() isn't called within `timeout` of the last kick
class IdleTimeoutWatchdog {
public:
    IdleTimeoutWatchdog(boost::beast::tcp_stream& stream, std::chrono::steady_clock::duration timeout)
        : stream_(stream), timeout_(timeout), thread_([this] { run(); }) {}

    IdleTimeoutWatchdog(const IdleTimeoutWatchdog&) = delete;
    IdleTimeoutWatchdog& operator=(const IdleTimeoutWatchdog&) = delete;

    ~IdleTimeoutWatchdog() {
        {
            std::lock_guard<std::mutex> lock(mutex_);
            done_ = true;
        }
        cv_.notify_one();
        thread_.join();
    }

    void kick() {
        std::lock_guard<std::mutex> lock(mutex_);
        deadline_ = std::chrono::steady_clock::now() + timeout_;
        cv_.notify_one();
    }

private:
    void run() {
        std::unique_lock<std::mutex> lock(mutex_);
        deadline_ = std::chrono::steady_clock::now() + timeout_;
        while (!done_) {
            cv_.wait_until(lock, deadline_);
            if (done_) {
                return;
            }
            if (std::chrono::steady_clock::now() >= deadline_) {
                boost::system::error_code ec;
                stream_.socket().close(ec);
                return;
            }
            // Spurious wake or kick() pushed the deadline out -- loop back
            // and wait on the (possibly updated) deadline_.
        }
    }

    boost::beast::tcp_stream& stream_;
    std::chrono::steady_clock::duration timeout_;
    std::mutex mutex_;
    std::condition_variable cv_;
    std::chrono::steady_clock::time_point deadline_;
    bool done_ = false;
    std::thread thread_;
};
