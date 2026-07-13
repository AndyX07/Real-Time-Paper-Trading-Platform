#include "engine/control/control_server.hpp"

ControlServiceImpl::ControlServiceImpl(SymbolRegistry& symbolRegistry) : symbolRegistry_(symbolRegistry) {}

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
