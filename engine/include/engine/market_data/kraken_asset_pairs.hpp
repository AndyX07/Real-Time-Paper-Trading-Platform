#pragma once

#include <optional>
#include <string_view>

struct SymbolPrecision {
    int pairDecimals;
    int lotDecimals;
};

std::optional<SymbolPrecision> fetchKrakenAssetPairPrecision(std::string_view symbol);
