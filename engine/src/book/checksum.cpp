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

namespace {

std::string formatAtNativePrecision(int64_t ticks, int64_t scale, int scaleDigits, int nativeDecimals) {
    std::string full = price_detail::formatFixedPoint(ticks, scale, scaleDigits);
    size_t dot = full.find('.');
    if (nativeDecimals == 0) {
        return full.substr(0, dot);
    }
    return full.substr(0, dot + 1 + static_cast<size_t>(nativeDecimals));
}

void appendLevels(std::string& buffer, std::span<const PriceLevel> levels, int priceDecimals, int quantityDecimals) {
    size_t n = std::min(levels.size(), CHECKSUM_DEPTH);
    for(size_t i = 0; i < n; i++) {
        buffer += checksum_detail::stripForChecksum(
            formatAtNativePrecision(levels[i].price.ticks, PRICE_SCALE, PRICE_SCALE_DIGITS, priceDecimals));
        buffer += checksum_detail::stripForChecksum(
            formatAtNativePrecision(levels[i].quantity.ticks, QUANTITY_SCALE, QUANTITY_SCALE_DIGITS, quantityDecimals));
    }
}

}

uint32_t computeBookChecksum(std::span<const PriceLevel> askLevels, std::span<const PriceLevel> bidLevels,
                              int priceDecimals, int quantityDecimals) {
    std::string buffer;
    appendLevels(buffer, askLevels, priceDecimals, quantityDecimals);
    appendLevels(buffer, bidLevels, priceDecimals, quantityDecimals);
    return checksum_detail::crc32(buffer);
}