#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <string_view>
#include <vector>

#include <simdjson.h>

#include "engine/book/price_level.hpp"

enum class BookMessageType { Snapshot, Update };

struct BookLevelUpdate {
    Price price;
    Quantity quantity;
};

struct BookMessage {
    std::string symbol;
    BookMessageType type;
    std::vector<BookLevelUpdate> bids;  // descending by price, best first
    std::vector<BookLevelUpdate> asks;  // ascending by price, best first
    uint32_t checksum;
};

std::optional<BookMessage> parseBookMessage(simdjson::ondemand::parser& parser, std::string_view raw);
