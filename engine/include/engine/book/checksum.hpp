#pragma once

#include <cstddef>
#include <cstdint>
#include <span>
#include <string>
#include <string_view>
#include "engine/book/price_level.hpp"

constexpr size_t CHECKSUM_DEPTH = 10;

namespace checksum_detail {

uint32_t crc32(std::string_view data);

std::string stripForChecksum(const std::string& decimalDigits);

}

uint32_t computeBookChecksum(std::span<const PriceLevel> askLevels, std::span<const PriceLevel> bidLevels,
                              int priceDecimals, int quantityDecimals);
