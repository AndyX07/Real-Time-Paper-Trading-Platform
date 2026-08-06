#pragma once

#include <cstdint>
#include <istream>
#include <optional>
#include <ostream>
#include <stdexcept>
#include <string>
#include <string_view>
#include <utility>
#include <vector>

constexpr uint32_t RECORDED_SESSION_MAGIC = 0x50545253; // "PTRS" reused as a file-format tag, distinct context
constexpr uint32_t RECORDED_SESSION_VERSION = 1;

struct RecordedFrame {
    uint64_t recvTsNanos;
    std::string raw;
};

inline void writeRecordedSessionHeader(std::ostream& out) {
    uint32_t magic = RECORDED_SESSION_MAGIC;
    uint32_t version = RECORDED_SESSION_VERSION;
    out.write(reinterpret_cast<const char*>(&magic), sizeof(magic));
    out.write(reinterpret_cast<const char*>(&version), sizeof(version));
}

inline bool readRecordedSessionHeader(std::istream& in) {
    uint32_t magic = 0;
    uint32_t version = 0;
    in.read(reinterpret_cast<char*>(&magic), sizeof(magic));
    in.read(reinterpret_cast<char*>(&version), sizeof(version));
    return in.good() && magic == RECORDED_SESSION_MAGIC && version == RECORDED_SESSION_VERSION;
}

inline void writeRecordedFrame(std::ostream& out, uint64_t recvTsNanos, std::string_view raw) {
    uint32_t payloadBytes = static_cast<uint32_t>(raw.size());
    out.write(reinterpret_cast<const char*>(&recvTsNanos), sizeof(recvTsNanos));
    out.write(reinterpret_cast<const char*>(&payloadBytes), sizeof(payloadBytes));
    out.write(raw.data(), static_cast<std::streamsize>(raw.size()));
}

// Returns std::nullopt at a clean end-of-stream; throws on a truncated
// record (a length prefix promising more bytes than the file actually
// has left).
inline std::optional<RecordedFrame> readRecordedFrame(std::istream& in) {
    uint64_t recvTsNanos = 0;
    uint32_t payloadBytes = 0;
    in.read(reinterpret_cast<char*>(&recvTsNanos), sizeof(recvTsNanos));
    if (in.eof()) {
        return std::nullopt;
    }
    in.read(reinterpret_cast<char*>(&payloadBytes), sizeof(payloadBytes));
    if (!in.good()) {
        throw std::runtime_error("recorded_session: truncated frame header");
    }

    std::string raw(payloadBytes, '\0');
    in.read(raw.data(), payloadBytes);
    if (!in.good()) {
        throw std::runtime_error("recorded_session: truncated frame payload");
    }
    return RecordedFrame{recvTsNanos, std::move(raw)};
}

inline std::vector<RecordedFrame> readAllRecordedFrames(std::istream& in) {
    if (!readRecordedSessionHeader(in)) {
        throw std::runtime_error("recorded_session: bad magic/version");
    }
    std::vector<RecordedFrame> frames;
    while (auto frame = readRecordedFrame(in)) {
        frames.push_back(std::move(*frame));
    }
    return frames;
}
