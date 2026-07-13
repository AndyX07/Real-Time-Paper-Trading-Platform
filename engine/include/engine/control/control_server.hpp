#pragma once

#include "engine/symbol/symbol_registry.hpp"
#include "paper_trader.grpc.pb.h"

class ControlServiceImpl final : public paper_trader::EngineControl::Service {
public:
    explicit ControlServiceImpl(SymbolRegistry& symbolRegistry);

    grpc::Status SubscribeBook(grpc::ServerContext* context, const paper_trader::SubscribeRequest* request,
                                paper_trader::SubscribeReply* response) override;
    grpc::Status UnsubscribeBook(grpc::ServerContext* context, const paper_trader::SubscribeRequest* request,
                                  paper_trader::SubscribeReply* response) override;

private:
    SymbolRegistry& symbolRegistry_;
};
