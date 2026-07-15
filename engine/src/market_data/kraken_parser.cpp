#include "engine/market_data/kraken_parser.hpp"

namespace {

BookLevelUpdate parseLevel(simdjson::ondemand::value level) {
    std::string_view priceToken = level["price"].raw_json_token();
    std::string_view qtyToken = level["qty"].raw_json_token();
    return BookLevelUpdate{
        Price::fromDecimalString(priceToken),
        Quantity::fromDecimalString(qtyToken),
    };
}

std::vector<BookLevelUpdate> parseLevels(simdjson::ondemand::array levels) {
    std::vector<BookLevelUpdate> result;
    for (auto level : levels) {
        result.push_back(parseLevel(level.value()));
    }
    return result;
}

}

std::optional<BookMessage> parseBookMessage(simdjson::ondemand::parser& parser, std::string_view raw) {
    simdjson::padded_string json(raw);
    simdjson::ondemand::document doc = parser.iterate(json);

    std::string_view channel;
    try {
        channel = doc["channel"].get_string().value();
    } catch (const simdjson::simdjson_error&) {
        return std::nullopt; // no "channel" field -- e.g. a subscribe ack, not a book message
    }
    if (channel != "book") {
        return std::nullopt;
    }

    std::string_view typeStr = doc["type"].get_string();
    BookMessageType type;
    if (typeStr == "snapshot") {
        type = BookMessageType::Snapshot;
    } else if (typeStr == "update") {
        type = BookMessageType::Update;
    } else {
        throw simdjson::simdjson_error(simdjson::INCORRECT_TYPE);
    }

    simdjson::ondemand::value entry = doc["data"].get_array().at(0);

    BookMessage message;
    message.type = type;
    message.symbol = std::string(entry["symbol"].get_string().value());
    message.bids = parseLevels(entry["bids"].get_array());
    message.asks = parseLevels(entry["asks"].get_array());
    message.checksum = static_cast<uint32_t>(entry["checksum"].get_uint64());

    return message;
}