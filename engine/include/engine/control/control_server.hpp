#pragma once

#include "engine/symbol/symbol_registry.hpp"
#include "paper_trader.grpc.pb.h"

class ControlServiceImpl final : public paper_trader::EngineControl::Service {
public:
    ControlServiceImpl(SymbolRegistry& symbolRegistry, uint64_t instanceId);

    grpc::Status SubscribeBook(grpc::ServerContext* context, const paper_trader::SubscribeRequest* request,
                                paper_trader::SubscribeReply* response) override;
    grpc::Status UnsubscribeBook(grpc::ServerContext* context, const paper_trader::SubscribeRequest* request,
                                  paper_trader::SubscribeReply* response) override;

    grpc::Status PlaceOrder(grpc::ServerContext* context, const paper_trader::PlaceOrderRequest* request,
                             paper_trader::OrderReply* response) override;
    grpc::Status CancelOrder(grpc::ServerContext* context, const paper_trader::CancelOrderRequest* request,
                              paper_trader::OrderReply* response) override;
    grpc::Status WatchFills(grpc::ServerContext* context, const paper_trader::WatchFillsRequest* request,
                             grpc::ServerWriter<paper_trader::FillEvent>* writer) override;

    grpc::Status GetEngineInfo(grpc::ServerContext* context, const paper_trader::EngineInfoRequest* request,
                                paper_trader::EngineInfoReply* response) override;

private:
    SymbolRegistry& symbolRegistry_;
    uint64_t instanceId_;
};
