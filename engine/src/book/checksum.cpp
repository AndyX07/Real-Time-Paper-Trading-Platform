#include <zlib.h>
#include "engine/book/checksum.hpp"

uint32_t checksum_detail::crc32(std::string_view data) {
    uLong crc = ::crc32(0L, Z_NULL, 0);
    crc = ::crc32(crc, reinterpret_cast<const Bytef*> (data.data()), data.size());
    return static_cast<uint32_t>(crc);
}

std::string checksum_detail::stripForChecksum(const std::string &decimalDigits) {
    std::string result = "";
    for(char c : decimalDigits) {
        if(c != '.') {
            result += c;
        }
    }
    size_t firstNonZero = result.find_first_not_of('0');
    if(firstNonZero == std::string::npos) {
        return "0";
    }
    return result.substr(firstNonZero);
}

void appendLevels(std::string& buffer, std::span<const PriceLevel> levels) {
    size_t n = std::min(levels.size(), CHECKSUM_DEPTH);
    for(size_t i = 0; i < n; i++) {
        buffer += checksum_detail::stripForChecksum(levels[i].price.toDecimalString());
        buffer += checksum_detail::stripForChecksum(levels[i].quantity.toDecimalString());
    }
}

uint32_t computeBookChecksum(std::span<const PriceLevel> askLevels, std::span<const PriceLevel> bidLevels) {
    std::string buffer;
    appendLevels(buffer, askLevels);
    appendLevels(buffer, bidLevels);
    return checksum_detail::crc32(buffer);
}