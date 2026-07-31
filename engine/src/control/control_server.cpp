#include "engine/control/control_server.hpp"

#include <chrono>

namespace {

constexpr std::chrono::milliseconds kWatchFillsPollInterval{500};

BookSide toBookSide(paper_trader::Side side) {
    return side == paper_trader::ASK ? BookSide::Ask : BookSide::Bid;
}

::OrderType toOrderType(paper_trader::OrderType type) {
    return type == paper_trader::LIMIT ? ::OrderType::Limit : ::OrderType::Market;
}

void fillOrderReply(const OrderResult& result, paper_trader::OrderReply* response) {
    response->set_accepted(result.accepted);
    response->set_engine_order_id(result.orderId);
    response->set_reject_reason(result.rejectReason);
    response->set_filled_size_ticks(result.filledSize.ticks);
}

}

ControlServiceImpl::ControlServiceImpl(SymbolRegistry& symbolRegistry, uint64_t instanceId)
    : symbolRegistry_{symbolRegistry}, instanceId_{instanceId} {}

grpc::Status ControlServiceImpl::SubscribeBook(grpc::ServerContext*, const paper_trader::SubscribeRequest* request,
                                                paper_trader::SubscribeReply* response) {
    SubscribeResult result = symbolRegistry_.subscribe(request->symbol());
    response->set_ok(result.ok);
    response->set_reason(result.reason);
    return grpc::Status::OK;
}

grpc::Status ControlServiceImpl::UnsubscribeBook(grpc::ServerContext*, const paper_trader::SubscribeRequest* request,
                                                  paper_trader::SubscribeReply* response) {
    SubscribeResult result = symbolRegistry_.unsubscribe(request->symbol());
    response->set_ok(result.ok);
    response->set_reason(result.reason);
    return grpc::Status::OK;
}

grpc::Status ControlServiceImpl::PlaceOrder(grpc::ServerContext*, const paper_trader::PlaceOrderRequest* request,
                                             paper_trader::OrderReply* response) {
    OrderResult result = symbolRegistry_.placeOrder(request->symbol(), toBookSide(request->side()),
                                                     toOrderType(request->type()), Price{request->price_ticks()},
                                                     Quantity{request->size_ticks()});
    fillOrderReply(result, response);
    return grpc::Status::OK;
}

grpc::Status ControlServiceImpl::CancelOrder(grpc::ServerContext*, const paper_trader::CancelOrderRequest* request,
                                              paper_trader::OrderReply* response) {
    OrderResult result = symbolRegistry_.cancelOrder(request->engine_order_id());
    fillOrderReply(result, response);
    return grpc::Status::OK;
}

grpc::Status ControlServiceImpl::WatchFills(grpc::ServerContext* context, const paper_trader::WatchFillsRequest*,
                                             grpc::ServerWriter<paper_trader::FillEvent>* writer) {
    while (!context->IsCancelled()) {
        Fill fill;
        if (!symbolRegistry_.waitForFill(fill, kWatchFillsPollInterval)) {
            continue; // nothing arrived within the poll interval -- recheck cancellation
        }

        paper_trader::FillEvent event;
        event.set_engine_order_id(fill.orderId);
        event.set_price_ticks(fill.price.ticks);
        event.set_size_ticks(fill.size.ticks);
        event.set_ts(fill.timestamp);

        if (!writer->Write(event)) {
            break; // client disconnected
        }
    }
    return grpc::Status::OK;
}

grpc::Status ControlServiceImpl::GetEngineInfo(grpc::ServerContext*, const paper_trader::EngineInfoRequest*,
                                                paper_trader::EngineInfoReply* response) {
    response->set_instance_id(instanceId_);
    return grpc::Status::OK;
}
